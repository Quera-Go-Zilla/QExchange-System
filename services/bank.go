package services

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"qexchange/database"
	"qexchange/models"

	"gorm.io/gorm"
)

const (
	callbackURL     = "http://localhost:8080/payment/verify"
	zarinpalRequest = "https://sandbox.banktest.ir/zarinpal/api.zarinpal.com/pg/v4/payment/request.json"
	zarinpalVerify  = "https://sandbox.banktest.ir/zarinpal/api.zarinpal.com/pg/v4/payment/verify.json"
	zarinpalGateURL = "https://sandbox.banktest.ir/zarinpal/www.zarinpal.com/pg/StartPay/"
)

type BankService interface {
	ChargeAccount(amount int, user models.User) (string, int, error) // returns payment_url, status code, error
	VerifyPayment(authority, status string) (int, error)             // returns status code, error
}

type bankService struct {
	db        *gorm.DB
	dbService database.DataBaseService
}

func NewBankService(db *gorm.DB) BankService {
	return &bankService{
		db:        db,
		dbService: database.NewDBService(db),
	}
}

type ZarinpalData struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Authority string `json:"authority"`
	FeeType   string `json:"fee_type"`
	Fee       int    `json:"fee"`
}

type ZarinpalResponse struct {
	Data   ZarinpalData  `json:"data"`
	Errors []interface{} `json:"errors"`
}

func (s *bankService) ChargeAccount(amount int, user models.User) (string, int, error) {
	bankRequestData := map[string]interface{}{
		"merchant_id":  os.Getenv("MerchantID"),
		"amount":       amount,
		"callback_url": callbackURL,
		"description":  "Payment to charge Qexchgange account",
	}

	jsonData, err := json.Marshal(bankRequestData)
	if err != nil {
		return "", http.StatusInternalServerError, errors.New("failed parsing request data")
	}

	// this line disables ssl check
	// should be removed in production and use http instead of client
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	res, err := client.Post(zarinpalRequest, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", http.StatusInternalServerError, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", http.StatusInternalServerError, err
	}

	var result ZarinpalResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", http.StatusInternalServerError, err
	}

	// add payment to database
	newPayment := models.PaymentInfo{
		UserID:    user.ID,
		Amount:    int64(amount),
		Status:    "Wait",
		Authority: result.Data.Authority,
	}

	createdPayment := s.db.Create(&newPayment)
	if createdPayment.Error != nil {
		return "", http.StatusInternalServerError, errors.New("failed to insert payment into database")
	}

	PaymentUrl := fmt.Sprintf("%v%v", zarinpalGateURL, result.Data.Authority)

	return PaymentUrl, http.StatusOK, nil
}

func (s *bankService) VerifyPayment(authority, status string) (int, error) {
	// find the payment
	var payment models.PaymentInfo
	dbResult := s.db.Where("authority = ?", authority).First(&payment)
	if dbResult.Error != nil {
		return http.StatusBadRequest, errors.New("no payment found")
	}

	if payment.Status != "Wait" {
		return http.StatusBadRequest, errors.New("no payment waiting for verification found")
	}

	if status == "NOK" {
		payment.Status = "Failed"
		dbResult = s.db.Save(&payment)
		if dbResult.Error != nil {
			return http.StatusInternalServerError, errors.New("faild updating payment record")
		}
		return http.StatusBadRequest, errors.New("payment verification failed")
	}

	data := map[string]interface{}{
		"merchant_id": os.Getenv("MerchantID"),
		"amount":      payment.Amount,
		"authority":   authority,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		payment.Status = "Failed"
		dbResult = s.db.Save(&payment)
		if dbResult.Error != nil {
			return http.StatusInternalServerError, errors.New("faild updating payment record")
		}
		return http.StatusInternalServerError, errors.New("faild parsing bank data to verify payment")
	}

	// this line disables ssl check
	// should be removed in production and use http instead of client
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	res, err := client.Post(zarinpalVerify, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		payment.Status = "Failed"
		dbResult = s.db.Save(&payment)
		if dbResult.Error != nil {
			return http.StatusInternalServerError, errors.New("faild updating payment record")
		}
		return http.StatusInternalServerError, errors.New("faild to send request to verify payment")
	}
	defer res.Body.Close()

	jsonBody := make(map[string]interface{})
	err = json.NewDecoder(res.Body).Decode(&jsonBody)
	if err != nil {
		payment.Status = "Failed"
		dbResult = s.db.Save(&payment)
		if dbResult.Error != nil {
			return http.StatusInternalServerError, errors.New("faild updating payment record")
		}
		return http.StatusInternalServerError, errors.New("faild to parse verification response")
	}

	if data, ok := jsonBody["data"]; ok {
		if dataMap, ok := data.(map[string]interface{}); ok {
			if code, ok := dataMap["code"]; ok {
				if code == float64(100) {
					// verified => update database
					payment.Status = "Successful"
					dbResult = s.db.Save(&payment)
					if dbResult.Error != nil {
						return http.StatusInternalServerError, errors.New("faild updating payment record")
					}

					// update user balance
					var user models.User
					result := s.db.Where("id = ?", payment.UserID).First(&user)
					if result.Error != nil {
						return http.StatusInternalServerError, errors.New("faild finsing user")
					}

					err = s.dbService.AddToUserBalanace(user, int(payment.Amount))
					if err != nil {
						return http.StatusInternalServerError, errors.New("faild updating user balance")
					}

					return http.StatusOK, nil

				} else if code == float64(101) {
					return http.StatusAlreadyReported, errors.New("payment already verified")
				} else {
					// failed => update database
					payment.Status = "Failed"
					dbResult = s.db.Save(&payment)
					if dbResult.Error != nil {
						return http.StatusInternalServerError, errors.New("faild updating payment record")
					}
					return http.StatusBadRequest, errors.New("code other than 100 or 101 returned")
				}
			} else {
				// failed => update database
				payment.Status = "Failed"
				dbResult = s.db.Save(&payment)
				if dbResult.Error != nil {
					return http.StatusInternalServerError, errors.New("faild updating payment record")
				}
				return http.StatusBadRequest, errors.New("no code in the json")
			}
		} else {
			// failed => update database
			payment.Status = "Failed"
			dbResult = s.db.Save(&payment)
			if dbResult.Error != nil {
				return http.StatusInternalServerError, errors.New("faild updating payment record")
			}
			return http.StatusBadRequest, errors.New("data in json failed")
		}
	} else {
		// failed => update database
		payment.Status = "Faield"
		dbResult = s.db.Save(&payment)
		if dbResult.Error != nil {
			return http.StatusInternalServerError, errors.New("faild updating payment record")
		}
		return http.StatusBadRequest, errors.New("no data in json")
	}
}