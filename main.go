package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"

	"bluemores/internal/config"
	"bluemores/internal/monitor"
	"bluemores/internal/store"
	"bluemores/internal/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	_ = godotenv.Load()

	exporterKey := os.Getenv("EXPORTER_KEY")
	passphrase := os.Getenv("DASHBOARD_PASSPHRASE")
	if exporterKey == "" && passphrase == "" {
		log.Fatal("nothing to run: set EXPORTER_KEY (exporter mode) and/or DASHBOARD_PASSPHRASE (monitor + dashboard mode)")
	}

	app := fiber.New(fiber.Config{AppName: "bluemores", DisableStartupMessage: true})
	app.Use(recover.New())
	if env("LOG_REQUESTS", "false") == "true" {
		app.Use(logger.New())
	}

	if exporterKey != "" {
		web.RegisterExporter(app, exporterKey, env("DISK_PATH", "/"))
		log.Println("exporter enabled: /mon/proc, /mon/mem, /mon/dfree")
	}

	var mon *monitor.Monitor
	if passphrase != "" {
		groups, err := config.Load(env("CONFIG_PATH", "config.json"))
		if err != nil {
			log.Fatal(err)
		}
		st, err := store.Open(env("DB_PATH", "bluemores.db"))
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		defer st.Close()

		timeout, err := time.ParseDuration(env("CHECK_TIMEOUT", "10s"))
		if err != nil {
			log.Fatalf("CHECK_TIMEOUT: %v", err)
		}
		days, _ := strconv.Atoi(env("RETENTION_DAYS", "30"))

		mon = monitor.New(groups, st, timeout, time.Duration(days)*24*time.Hour, env("RUN_ON_START", "true") == "true")
		if err := mon.Start(); err != nil {
			log.Fatal(err)
		}
		web.RegisterDashboard(app, passphrase, env("COOKIE_SECURE", "false") == "true", mon, st)
		log.Println("monitor + dashboard enabled")
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
		<-quit
		log.Println("shutting down")
		if mon != nil {
			mon.Stop()
		}
		_ = app.Shutdown()
	}()

	addr := ":" + env("PORT", "3000")
	log.Printf("listening on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatal(err)
	}
}
