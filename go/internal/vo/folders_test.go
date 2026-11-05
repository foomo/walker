package vo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFolders(t *testing.T) {
	results := map[string]ScrapeResult{}
	for targetURL, code := range map[string]int{
		"https://example.com/":                  200,
		"https://example.com/about":             200,
		"https://example.com/shop/":             200,
		"https://example.com/shop/a?color=red":  200,
		"https://example.com/shop/shoes":        200, // the folder page of shoes
		"https://example.com/shop/shoes/b":      404,
		"https://example.com/shop/shoes/c#top":  0,
		"https://example.com/shopping/d":        200,
		"http://example.com/legacy/e":           200,
		"https://user:pw@other.org:8080/blog/x": 500,
	} {
		results[targetURL] = ScrapeResult{TargetURL: targetURL, Code: code}
	}

	folders := Folders(results)
	if !assert.Len(t, folders, 3, "http and https are separate roots") {
		return
	}
	var example, other Folder
	for _, f := range folders {
		switch f.Prefix {
		case "https://example.com":
			example = f
		case "https://user:pw@other.org:8080":
			other = f
		}
	}

	assert.Equal(t, "example.com", example.Name)
	assert.Equal(t, 8, example.Pages)
	assert.Equal(t, 2, example.Errors)

	names := []string{}
	for _, f := range example.Folders {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"shop", "shopping"}, names, "pages like /about are not folders, /shop and /shopping are separate")

	shop := example.Folders[0]
	assert.Equal(t, "https://example.com/shop", shop.Prefix, "no trailing slash")
	assert.Equal(t, 5, shop.Pages, "the /shop/ index page, a, the shoes folder page and the two shoes pages")
	assert.Equal(t, 2, shop.Errors)
	if assert.Len(t, shop.Folders, 1) {
		shoes := shop.Folders[0]
		assert.Equal(t, "https://example.com/shop/shoes", shoes.Prefix)
		assert.Equal(t, 3, shoes.Pages, "the folder page /shop/shoes counts into its folder")
		assert.Nil(t, shoes.Folders, "leaves have no children - not an empty list")
	}

	assert.Equal(t, "other.org:8080", other.Name, "credentials are not shown")

	// every folder counts exactly the target urls InFolder matches
	var check func(f Folder)
	check = func(f Folder) {
		count := 0
		for targetURL := range results {
			if InFolder(targetURL, f.Prefix) {
				count++
			}
		}
		assert.Equal(t, f.Pages, count, f.Prefix)
		for _, c := range f.Folders {
			check(c)
		}
	}
	for _, f := range folders {
		check(f)
	}
}

func TestInFolder(t *testing.T) {
	for targetURL, want := range map[string]bool{
		"https://example.com/shop":         true,
		"https://example.com/shop/":        true,
		"https://example.com/shop/a":       true,
		"https://example.com/shop?page=2":  true,
		"https://example.com/shop#reviews": true,
		"https://example.com/shopping":     false,
		"https://example.com/sho":          false,
	} {
		assert.Equal(t, want, InFolder(targetURL, "https://example.com/shop"), targetURL)
	}
}
