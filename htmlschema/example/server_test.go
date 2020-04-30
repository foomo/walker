package example

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
)

func getExampleDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename))
}

func TestServer(t *testing.T) {
	s := NewServer(filepath.Join(getExampleDir(), "htdocs"))
	testServer := httptest.NewServer(s)
	defer testServer.Close()
	fmt.Println(testServer.URL)
	htmlBytes, group, errCall := call(testServer, "/content/page-a.html")
	fmt.Println("group:", group, string(htmlBytes), errCall)
}

func call(testServer *httptest.Server, path string) (htmlBytes []byte, group string, err error) {
	resp, errGet := http.Get(testServer.URL + path)
	if errGet != nil {
		return nil, "", errGet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("unexpectded status: " + resp.Status)
	}
	defer resp.Body.Close()
	htmlBytes, errRead := ioutil.ReadAll(resp.Body)
	if errRead != nil {
		return nil, "", errRead
	}
	return htmlBytes, resp.Header.Get("group"), nil
}
