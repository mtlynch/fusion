package api

import (
	"net/http"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

type Session struct{}

func (s Session) Create(c echo.Context, password string, useSecureCookie bool) error {
	var req struct {
		Password string `json:"password" validate:"required"`
	}

	if err := bindAndValidate(&req, c); err != nil {
		return err
	}

	if req.Password != password {
		return echo.NewHTTPError(http.StatusUnauthorized, "Wrong password")
	}

	sess, _ := session.Get("login", c)

	if !useSecureCookie {
		sess.Options.Secure = false
		sess.Options.SameSite = http.SameSiteDefaultMode
	}

	sess.Values["password"] = password
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusCreated)
}

func (s Session) Check(c echo.Context) (bool, error) {
	if _, err := session.Get("login", c); err != nil {
		return false, err
	}
	return true, nil
}

func (s Session) Delete(c echo.Context) error {
	sess, err := session.Get("login", c)
	if err != nil {
		return err
	}
	sess.Values["password"] = ""
	sess.Options.MaxAge = -1
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusNoContent)
}
