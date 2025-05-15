package routes

import (
	"backend/controllers"
	middlewares "backend/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterCategoryRoutes(app *fiber.App) {
	category := app.Group("/categories", middlewares.JWTMiddleware)
	category.Get("/", controllers.GetCategoriesByMarket)

	api := app.Group("/api")

	api.Get("/categories", controllers.GetCategories)
	api.Get("/categories/:id", controllers.GetCategoryByID)
	api.Post("/categories", controllers.CreateCategory)
	api.Put("/categories/:id", controllers.UpdateCategory)
	api.Delete("/categories/:id", controllers.DeleteCategory)
	api.Get("/categories/market/:market_id", controllers.GetCategoriesByMarketID)

}
