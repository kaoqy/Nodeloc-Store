package domain

import "github.com/kaoqy/Nodeloc-Store/internal/models"

// Catalog entities reuse the canonical persistence models so the module remains
// compatible with the application's existing GORM schema and migrations.
type Product = models.Product
type Card = models.Card
type Category = models.Category
type Coupon = models.Coupon

const (
	ProductTypeCard   = "card"
	ProductTypeManual = "manual"

	CardStatusAvailable = "available"
	CardStatusSold      = "sold"
	CardStatusDisabled  = "disabled"
)
