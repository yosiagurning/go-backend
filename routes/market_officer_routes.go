package routes

import (
	"backend/controllers"
	"backend/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterMarketOfficerRoutes(app *fiber.App) {
	api := app.Group("/api/market-officers")

	api.Get("/", controllers.GetMarketOfficers)         // Ambil semua petugas pasar
	api.Get("/:id", controllers.GetMarketOfficerByID)   // Ambil petugas pasar berdasarkan ID
	api.Post("/", controllers.CreateMarketOfficer)      // Tambah petugas pasar baru
	api.Put("/:id", controllers.UpdateMarketOfficer)    // Perbarui data petugas pasar
	api.Delete("/:id", controllers.DeleteMarketOfficer) // Hapus petugas pasar
}

func OfficerRoutes(app *fiber.App) {
	app.Patch("/api/officers/:id/toggle", controllers.ToggleOfficerStatus)
}

func SetupRoutes(app *fiber.App) {
	officerRoutes := app.Group("/officers")
	officerRoutes.Post("/", controllers.CreateMarketOfficer)

	api := app.Group("/api")

	// Butuh autentikasi
	protected := api.Group("/protected", middleware.JWTMiddleware)
	protected.Get("/categories", controllers.GetCategories)
	protected.Post("/categories", controllers.CreateCategory)
	protected.Put("/categories/:id", controllers.UpdateCategory)
	protected.Delete("/categories/:id", controllers.DeleteCategory)
}
func MarketOfficer(app *fiber.App) {
	auth := app.Group("/auth")
	auth.Post("/login", controllers.Login)
}
