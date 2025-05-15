package routes

import (
	"backend/controllers"

	"github.com/gofiber/fiber/v2"
)

func RegisterPriceRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/prices/chart/:id", controllers.GetPriceHistory)
	api.Get("/price-histories/:item_id", controllers.GetPriceHistoryByItem)
	api.Get("/price-histories/category/:category_id", controllers.GetPriceHistoryByCategory)

	api.Get("/prices", controllers.GetPrices)
	api.Get("/prices/:id", controllers.GetPriceByID)
	api.Post("/prices", controllers.CreatePrice)
	api.Put("/prices/:id", controllers.UpdatePrice)
	api.Delete("/prices/:id", controllers.DeletePrice)

	api.Get("/dashboard-data", controllers.GetDashboardData)
}
