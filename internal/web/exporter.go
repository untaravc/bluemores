package web

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"

	"bluemores/internal/metrics"
)

// RegisterExporter mounts the /mon/* endpoints, protected by a bearer key.
func RegisterExporter(app *fiber.App, key, diskPath string) {
	mon := app.Group("/mon", bearerAuth(key))
	mon.Get("/proc", reading(metrics.Processor))
	mon.Get("/mem", reading(metrics.Memory))
	mon.Get("/dfree", reading(func() (metrics.Reading, error) { return metrics.DiskFree(diskPath) }))
}

func bearerAuth(key string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, ok := strings.CutPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(key)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": false, "message": "unauthorized"})
		}
		return c.Next()
	}
}

func reading(fn func() (metrics.Reading, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		r, err := fn()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"status": false, "message": err.Error()})
		}
		return c.JSON(fiber.Map{"status": true, "data": r})
	}
}
