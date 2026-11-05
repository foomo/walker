package demoshop

type Feature struct {
	Label       string
	Description string
}

type FeatureSection struct {
	Label    string
	Features []Feature
}

type Product struct {
	ID          string
	Name        string
	Description string
	// Price in cents
	Price    int
	Currency string
	// Brand defaults to the brand of the category
	Brand           string
	FeatureSections []FeatureSection
	// Image, SocialImage and CategoryID are set by the loader - SocialImage
	// is a png for OpenGraph, social networks do not render svg
	Image       string `json:"-"`
	SocialImage string `json:"-"`
	CategoryID  string `json:"-"`
}

type Category struct {
	ID          string
	Name        string
	Description string
	Brand       string
	Products    []Product
}

type Directory struct {
	Categories []Category
}
