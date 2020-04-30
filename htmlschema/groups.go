package htmlschema

import (
	"errors"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type GroupValidator struct {
	validators       map[string]map[string]*Schema
	defaultValidator *Schema
}

func NewGroupValidator(root string) (groupValidator *GroupValidator, err error) {
	groupDirs, errReadGroupDirs := ioutil.ReadDir(root)
	if errReadGroupDirs != nil {
		return nil, errReadGroupDirs
	}
	groupValidator = &GroupValidator{
		validators: map[string]map[string]*Schema{},
	}
	for _, groupDir := range groupDirs {
		if groupDir.IsDir() && !strings.HasPrefix(groupDir.Name(), ".") {
			groupValidator.validators[groupDir.Name()] = map[string]*Schema{}
			groupFilePath := filepath.Join(root, groupDir.Name())
			schemaFiles, errReadSchemaDir := ioutil.ReadDir(groupFilePath)
			if errReadSchemaDir != nil {
				return nil, errReadSchemaDir
			}
			for _, schemaFile := range schemaFiles {
				if schemaFile.IsDir() || strings.HasPrefix(schemaFile.Name(), ".") {
					continue
				}
				schema, errSchema := load(filepath.Join(groupFilePath, schemaFile.Name()), nil)
				if errSchema != nil {
					return nil, errSchema
				}
				groupValidator.validators[groupDir.Name()][strings.TrimSuffix(schemaFile.Name(), ".html")] = schema
			}
		} else if groupDir.Name() == "default.html" {
			defaultSchema, errDefaultSchema := load(filepath.Join(root, groupDir.Name()), nil)
			if errDefaultSchema != nil {
				return nil, errDefaultSchema
			}
			groupValidator.defaultValidator = defaultSchema
		}
	}
	return
}

func (gv *GroupValidator) getSchemaForGroup(group string) (schema *Schema) {
	if group == "default" && gv.defaultValidator != nil {
		return gv.defaultValidator
	}
	for groupRoot, groupSchemata := range gv.validators {
		for groupPage, schema := range groupSchemata {
			if group == groupRoot+"/"+groupPage {
				return schema
			}
		}
	}
	return nil
}

func (gv *GroupValidator) Validate(group string, htmlBytes []byte, w io.Writer) (r *Report, err error) {
	schema := gv.getSchemaForGroup(group)
	if schema == nil {
		return nil, errors.New("could not find schema for " + group)
	}
	return schema.Validate(htmlBytes, w)
}
