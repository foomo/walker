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

type Target struct {
	BaseURL string
	Paths   []string
}
type Config struct {
	Concurrency       int
	Addr              string
	Target            Target
	Ignore            []string
	IgnoreQueriesWith []string
	IgnoreAllQueries  bool
	UseCookies        bool
	Depth             int
	Paging            bool
	IgnoreRobots      bool
}

func Get(filename string) (conf *Config, err error) {
	conf = &Config{
		Concurrency:      2,
		UseCookies:       true,
		IgnoreAllQueries: false,
		IgnoreRobots:     false,
	}
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
