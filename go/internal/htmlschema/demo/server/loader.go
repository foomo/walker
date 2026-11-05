package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// productsDir holds one directory per category: category.json, and per
// product <id>-<name>.json with its image <id>-<name>.svg and a png of it for
// OpenGraph - rendered with: rsvg-convert -w 600 -h 600 -o x.png x.svg
const productsDir = "catalogue/products"

// Load reads the catalogue from fsys
func Load(fsys fs.FS) (*Directory, error) {
	entries, err := fs.ReadDir(fsys, productsDir)
	if err != nil {
		return nil, err
	}
	directory := &Directory{}
	for _, entry := range entries {
		// hidden directories - editor or tool state - are no categories, go:embed skips them too
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		category, err := loadCategory(fsys, path.Join(productsDir, entry.Name()), entry.Name())
		if err != nil {
			return nil, err
		}
		directory.Categories = append(directory.Categories, *category)
	}
	sort.Slice(directory.Categories, func(i, j int) bool { return directory.Categories[i].ID < directory.Categories[j].ID })
	return directory, nil
}

func loadCategory(fsys fs.FS, dir, id string) (*Category, error) {
	category := &Category{ID: id, Name: id}
	if err := readJSON(fsys, path.Join(dir, "category.json"), category); err != nil {
		return nil, err
	}
	category.ID = id
	files, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || name == "category.json" || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		product := Product{}
		if err := readJSON(fsys, path.Join(dir, name), &product); err != nil {
			return nil, err
		}
		if product.ID == "" || !strings.HasPrefix(name, product.ID+"-") {
			return nil, fmt.Errorf("%s: the file name has to start with the product id %q", path.Join(dir, name), product.ID)
		}
		product.CategoryID = id
		if product.Brand == "" {
			product.Brand = category.Brand
		}
		base := strings.TrimSuffix(name, ".json")
		if _, err := fs.Stat(fsys, path.Join(dir, base+".svg")); err == nil {
			product.Image = "/images/" + id + "/" + base + ".svg"
		}
		if _, err := fs.Stat(fsys, path.Join(dir, base+".png")); err == nil {
			product.SocialImage = "/images/" + id + "/" + base + ".png"
		}
		category.Products = append(category.Products, product)
	}
	sort.Slice(category.Products, func(i, j int) bool { return category.Products[i].ID < category.Products[j].ID })
	return category, nil
}

func readJSON(fsys fs.FS, name string, v any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Product returns the product with id
func (d *Directory) Product(id string) (*Product, *Category) {
	for ci := range d.Categories {
		for pi := range d.Categories[ci].Products {
			if d.Categories[ci].Products[pi].ID == id {
				return &d.Categories[ci].Products[pi], &d.Categories[ci]
			}
		}
	}
	return nil, nil
}

// Category returns the category with id
func (d *Directory) Category(id string) *Category {
	for i := range d.Categories {
		if d.Categories[i].ID == id {
			return &d.Categories[i]
		}
	}
	return nil
}
