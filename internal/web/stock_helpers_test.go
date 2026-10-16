package web

import (
	"net/http"
	"strconv"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Помощники тестов склада: данные создаются через API справочников.

type stkID struct {
	ID int64 `json:"id"`
}

// stkSignIn создаёт пользователя и возвращает вошедший клиент.
func stkSignIn(e *env, login string) *client {
	e.t.Helper()
	e.createUser(login, "password-"+login, false)
	c := e.client()
	c.login(login, "password-"+login)
	return c
}

func stkCreate(c *client, target string, body any) int64 {
	c.e.t.Helper()
	rec := c.do(http.MethodPost, target, body)
	expect(c.e.t, rec, http.StatusCreated, "создание "+target)
	return decode[stkID](c.e.t, rec).ID
}

// stkCategory — своя категория пользователя.
func stkCategory(c *client, name string) int64 {
	return stkCreate(c, "/api/categories", map[string]any{"name": name, "color": "blue"})
}

func stkProduct(c *client, category int64, name, unit string, size int64) int64 {
	return stkCreate(c, "/api/products", map[string]any{
		"category_id": category, "name": name, "unit": unit, "package_size": size})
}

func stkLocation(c *client, name string) int64 {
	return stkCreate(c, "/api/locations", map[string]any{"name": name})
}

type stkPurchaseBody struct {
	Purchase struct {
		ID    int64   `json:"id"`
		Total int64   `json:"total"`
		Store *string `json:"store"`
		Lines []struct {
			ProductID int64   `json:"product_id"`
			Product   string  `json:"product"`
			UnitIDs   []int64 `json:"unit_ids"`
		} `json:"lines"`
	} `json:"purchase"`
}

// stkBuy покупает упаковки позиции и возвращает id экземпляров.
func stkBuy(c *client, product, packages, price int64, location *int64) []int64 {
	c.e.t.Helper()
	line := map[string]any{"product_id": product, "packages": packages, "price_per_package": price}
	if location != nil {
		line["location_id"] = *location
	}
	rec := c.do(http.MethodPost, "/api/purchases", map[string]any{"date": "2026-10-02", "lines": []any{line}})
	expect(c.e.t, rec, http.StatusCreated, "покупка")
	return decode[stkPurchaseBody](c.e.t, rec).Purchase.Lines[0].UnitIDs
}

type stkConsumeBody struct {
	Movements       []catalog.Movement     `json:"movements"`
	Product         catalog.ProductSummary `json:"product"`
	OpenedUnitIDs   []int64                `json:"opened_unit_ids"`
	FinishedUnitIDs []int64                `json:"finished_unit_ids"`
	Cost            *int64                 `json:"cost"`
}

type stkShortageBody struct {
	Error     string `json:"error"`
	Available int64  `json:"available"`
}

func stkIDs(ms []catalog.Movement) []int64 {
	out := make([]int64, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func stkPath(prefix string, id int64, suffix string) string {
	return prefix + strconv.FormatInt(id, 10) + suffix
}
