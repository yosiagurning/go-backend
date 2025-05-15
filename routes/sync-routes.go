package routes

import (
	"backend/controllers"

	"github.com/gofiber/fiber/v2"
)

func RegisterSyncRoutes(app *fiber.App) {
	api := app.Group("/api")
	api.Get("/sync", controllers.SyncBarangAndPrice)
}
