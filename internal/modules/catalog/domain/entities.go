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

	// DeliveryChannel names the product's delivery channel. Card and manual are
	// the shop's own fulfilment; NewAPI is the product-level New-API redemption
	// channel, which creates a code after payment instead of drawing on stock.
	DeliveryChannelCard   = "card"
	DeliveryChannelManual = "manual"
	DeliveryChannelNewAPI = "new_api"

	CardStatusAvailable = "available"
	CardStatusSold      = "sold"
	CardStatusDisabled  = "disabled"
)
