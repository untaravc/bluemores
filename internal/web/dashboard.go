package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"bluemores/internal/monitor"
	"bluemores/internal/store"
)

//go:embed templates/*.html
var templates embed.FS

const (
	cookieName = "bluemores_session"
	sessionTTL = 7 * 24 * time.Hour
)

type Dashboard struct {
	passphrase string
	secret     []byte
	secure     bool
	mon        *monitor.Monitor
	store      *store.Store
}

// RegisterDashboard mounts the login page, dashboard UI and its JSON API.
func RegisterDashboard(app *fiber.App, passphrase string, secureCookie bool, mon *monitor.Monitor, st *store.Store) {
	sum := sha256.Sum256([]byte("bluemores-session:" + passphrase))
	d := &Dashboard{passphrase: passphrase, secret: sum[:], secure: secureCookie, mon: mon, store: st}

	app.Get("/login", d.loginPage)
	app.Post("/login", limiter.New(limiter.Config{Max: 10, Expiration: time.Minute}), d.login)
	app.Get("/logout", d.logout)

	app.Get("/", d.requireSession(false), page("templates/dashboard.html"))
	api := app.Group("/api", d.requireSession(true))
	api.Get("/overview", d.overview)
	api.Get("/history/:id", d.history)
	api.Post("/check/:id", d.checkNow)
}

func page(name string) fiber.Handler {
	html, err := templates.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return func(c *fiber.Ctx) error {
		c.Type("html", "utf-8")
		return c.Send(html)
	}
}

func (d *Dashboard) sign(exp int64) string {
	mac := hmac.New(sha256.New, d.secret)
	mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (d *Dashboard) validSession(c *fiber.Ctx) bool {
	expStr, sig, ok := strings.Cut(c.Cookies(cookieName), ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(d.sign(exp)))
}

func (d *Dashboard) requireSession(api bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if d.validSession(c) {
			return c.Next()
		}
		if api {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"status": false, "message": "unauthorized"})
		}
		return c.Redirect("/login")
	}
}

func (d *Dashboard) loginPage(c *fiber.Ctx) error {
	if d.validSession(c) {
		return c.Redirect("/")
	}
	return page("templates/login.html")(c)
}

func (d *Dashboard) login(c *fiber.Ctx) error {
	given := c.FormValue("passphrase")
	if subtle.ConstantTimeCompare([]byte(given), []byte(d.passphrase)) != 1 {
		return c.Redirect("/login?error=1")
	}
	exp := time.Now().Add(sessionTTL).Unix()
	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    strconv.FormatInt(exp, 10) + "." + d.sign(exp),
		Expires:  time.Unix(exp, 0),
		HTTPOnly: true,
		Secure:   d.secure,
		SameSite: "Lax",
		Path:     "/",
	})
	return c.Redirect("/")
}

func (d *Dashboard) logout(c *fiber.Ctx) error {
	c.ClearCookie(cookieName)
	return c.Redirect("/login")
}

type appView struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Source    string        `json:"source"`
	Schedule  string        `json:"schedule"`
	Threshold string        `json:"threshold"`
	Latest    *store.Result `json:"latest"`
}

type groupView struct {
	Group string    `json:"group"`
	Apps  []appView `json:"apps"`
}

func (d *Dashboard) overview(c *fiber.Ctx) error {
	latest, err := d.store.Latest()
	if err != nil {
		return err
	}
	out := []groupView{}
	for _, g := range d.mon.Groups() {
		gv := groupView{Group: g.Group, Apps: []appView{}}
		for _, a := range g.Apps {
			av := appView{ID: a.ID, Name: a.Name, Type: a.Type, Source: a.Source, Schedule: a.Schedule, Threshold: a.Threshold}
			if r, ok := latest[a.ID]; ok {
				av.Latest = &r
			}
			gv.Apps = append(gv.Apps, av)
		}
		out = append(out, gv)
	}
	return c.JSON(fiber.Map{"status": true, "data": out, "now": time.Now()})
}

func (d *Dashboard) history(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, ok := d.mon.FindApp(id); !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"status": false, "message": "unknown app"})
	}
	hours := c.QueryInt("hours", 24)
	if hours < 1 || hours > 24*90 {
		hours = 24
	}
	rows, err := d.store.History(id, time.Now().Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"status": true, "data": rows})
}

func (d *Dashboard) checkNow(c *fiber.Ctx) error {
	app, ok := d.mon.FindApp(c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"status": false, "message": "unknown app"})
	}
	return c.JSON(fiber.Map{"status": true, "data": d.mon.Check(app)})
}
