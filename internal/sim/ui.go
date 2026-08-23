package sim

import (
	_ "embed"

	"github.com/gofiber/fiber/v3"
)

//go:embed ui.html
var uiHTML string

// RegisterUI serves the local simulation configuration page.
func RegisterUI(app *fiber.App) {
	app.Get("/", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(uiHTML)
	})
}
