package domain

// ProductQuery is the storefront's read filter. The buyer SPA used to fetch
// every published product and sort it in the browser, which meant a shop with a
// real catalogue shipped its whole inventory to every visitor and had no way to
// rank by sales at all.
type ProductQuery struct {
	PublishedOnly bool
	CategoryID    *uint
	FeaturedOnly  bool
	InStockOnly   bool
	Search        string
	Sort          string
	Limit         int
	Offset        int
}

// Sort orders the storefront understands. Leaving it empty keeps the shop
// owner's manual ordering; anything else is refused by the service rather than
// silently re-ranked, so a typo in a link shows as an error instead of a page
// that quietly looks wrong.
const (
	SortDefault   = ""
	SortSalesDesc = "sales"
	SortPriceAsc  = "price_asc"
	SortPriceDesc = "price_desc"
	SortNewest    = "newest"
)

// ValidSort reports whether the caller asked for a known ordering.
func ValidSort(value string) bool {
	switch value {
	case SortDefault, SortSalesDesc, SortPriceAsc, SortPriceDesc, SortNewest:
		return true
	}
	return false
}

// CategoryView carries the buyer-facing count of purchasable products, which is
// what a category chip on the home page needs to show.
type CategoryView struct {
	Category
	ProductCount int64 `json:"product_count"`
}

// StoreStats is the home page's number row.
type StoreStats struct {
	Products   int64 `json:"products"`
	Stock      int64 `json:"stock"`
	Sales      int64 `json:"sales"`
	Categories int64 `json:"categories"`
}

// CardFilter narrows the back office's card inventory view.
type CardFilter struct {
	ProductID *uint
	Status    string
	Search    string
	Limit     int
	Offset    int
}
