package routes

import (
	"backend/controllers"

	"github.com/gofiber/fiber/v2"
)

func RegisterMarketRoutes(app *fiber.App) {
	api := app.Group("/api")

	api.Get("/markets", controllers.GetMarkets)            // Ambil semua pasar
	api.Get("/markets/:id", controllers.GetMarketByID)     // Ambil pasar berdasarkan ID
	api.Post("/markets", controllers.CreateMarket)         // Tambah pasar baru
	api.Put("/markets/:id", controllers.UpdateMarket)      // Update pasar
	api.Put("/markets/:id/location", controllers.UpdateMarketLocation) // Perbaiki lokasi pasar
	api.Delete("/markets/:id", controllers.DeleteMarket)   // Hapus pasar
}
