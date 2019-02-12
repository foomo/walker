package config

import (
	"io/ioutil"

	yaml "gopkg.in/yaml.v1"
)

type CheckContent struct {
	Title       bool
	Description bool
	Tags        []string
}

type Group struct {
	Routes       []string
	CheckContent *CheckContent
}

type Config struct {
	Frontend   string
	Addr       string
	Target     string
	Groups     []*Group
	Ignore     []string
	UseCookies bool
}

func Get(filename string) (conf *Config, err error) {
	yamlBytes, errRead := ioutil.ReadFile(filename)
	if errRead != nil {
		err = errRead
		return
	}
	errUnmarshal := yaml.Unmarshal(yamlBytes, &conf)
	if errUnmarshal != nil {
		err = errUnmarshal
		return
	}
	return
}
