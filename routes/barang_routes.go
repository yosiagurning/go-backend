package routes

import (
	"backend/controllers"

	"github.com/gofiber/fiber/v2"
)

func RegisterBarangRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/barang", controllers.GetAllBarang)
	api.Get("/barang/:id", controllers.GetBarangByID)
	api.Post("/barang", controllers.CreateBarang)
	api.Put("/barang/:id", controllers.UpdateBarang)
	api.Delete("/barang/:id", controllers.DeleteBarang)
	api.Get("/barang/:id/history", controllers.GetBarangHistory)
	app.Get("/api/barang/market/:marketId", controllers.GetBarangByMarketID)
	app.Get("/api/barang/market/:marketId/paginated", controllers.GetBarangByMarketIDPaginated)
}
