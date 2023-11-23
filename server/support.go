package server

import (
	"qexchange/handlers"
	"qexchange/middlewares"
	"qexchange/services"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func SupportRoutes(e *echo.Echo, db *gorm.DB) {
	SupportService := services.NewSupportService(db)
	e.POST("/support/send-ticket", handlers.SendTicket(SupportService), middlewares.AuthMiddleware(db))
}