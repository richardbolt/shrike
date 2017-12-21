package store

import (
	"fmt"
	"hash/adler32"
	"net/url"
	"strings"

	toxy "github.com/Shopify/toxiproxy/client"
	"github.com/armon/go-radix"
)

// New returns a store of proxies.
// Proxies are stored by path even though proxies are named sans forward slashes.
func New(root url.URL, sep string) *ProxyStore {
	return &ProxyStore{
		root: root,
		sep:  sep,
		tree: radix.New(),
	}
}

// ProxyStore stores our proxies in an efficient fashion for path prefix matching
type ProxyStore struct {
	root url.URL
	sep  string
	tree *radix.Tree
}

// Add a proxy
func (s *ProxyStore) Add(proxy *toxy.Proxy) {
	s.tree.Insert(PathNameFrom(s.sep, proxy.Name), proxy)
}

// Get a proxy by path prefix
func (s *ProxyStore) Get(path string) *toxy.Proxy {
	p, m := s.tree.Get(path)
	if m == false {
		return nil
	}
	return p.(*toxy.Proxy)
}

// Delete a proxy
func (s *ProxyStore) Delete(proxy *toxy.Proxy) {
	s.tree.Delete(PathNameFrom(s.sep, proxy.Name))
}

// ToMap returns the Proxy store entries as a map of proxies
func (s *ProxyStore) ToMap() map[string]*toxy.Proxy {
	proxies := map[string]*toxy.Proxy{}
	for k, v := range s.tree.ToMap() {
		proxies[k] = v.(*toxy.Proxy)
	}
	return proxies
}

// Match returns a url.URL and a boolean to indicate whether we matched or are using the default.
func (s *ProxyStore) Match(path string) (url.URL, bool) {
	_, p, m := s.tree.LongestPrefix(path)
	if !m {
		return s.root, false
	}
	proxy := p.(*toxy.Proxy)
	// Hard coding some http in here. TODO: see if we can make this work HTTPS as well.
	if u, err := url.Parse(fmt.Sprintf("http://%s", proxy.Listen)); err == nil {
		return *u, true
	}
	return s.root, false
}

// NumFrom string to map a path or proxy name to a port number.
// The number returned will be in the range min < x < 65536
func NumFrom(s string) uint16 {
	var min uint16 = 10000
	x := uint16(adler32.Checksum([]byte(s)) / 65536)
	if x < min {
		for {
			x = x * 2
			if x > min {
				break
			}
		}
	}
	return x
}

// ProxyNameFrom returns the proxy name normalized from str
func ProxyNameFrom(sep, str string) string {
	return strings.Replace(str, "/", sep, -1)
}

// PathNameFrom returns the path name from proxy name str
func PathNameFrom(sep, str string) string {
	return strings.Replace(str, sep, "/", -1)
}
