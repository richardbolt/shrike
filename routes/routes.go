package routes

import (
	"fmt"
	"hash/adler32"
	"net/url"
	"strings"

	toxy "github.com/Shopify/toxiproxy/client"
	"github.com/armon/go-radix"
)

// Config information.
type Config struct {
	// DownstreamProxyURL is where we proxy requests and where we route requests via the Toxy.
	DownstreamProxyURL url.URL
	// ToxyAddress is the ip or DNS address of your ToxyProxy instance.
	ToxyAddress string
	// ToxyNamePathSeparator is the string used to sub with / for storing path info sans slashes in Toxiproxy
	ToxyNamePathSeparator string
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

// ProxyStore stores our proxies in an efficient fashion for path prefix matching
type ProxyStore struct {
	client *toxy.Client
	root   url.URL
	sep    string
	tree   *radix.Tree
}

// Add a proxy at path
func (s *ProxyStore) Add(proxy *toxy.Proxy) {
	s.tree.Insert(PathNameFrom(s.sep, proxy.Name), proxy)
}

// Match returns a url.URL and a boolean to indicate whether we matched or are using the default.
func (s *ProxyStore) Match(path string) (url.URL, bool) {
	_, p, m := s.tree.LongestPrefix(path)
	if !m {
		return s.root, false
	}
	proxy := p.(*toxy.Proxy)
	if u, err := url.Parse(fmt.Sprintf("http://%s", proxy.Listen)); err == nil {
		return *u, true
	}
	return s.root, false
}

// Populate the store with the proxy information and matchers
func (s *ProxyStore) Populate() error {
	proxies, err := s.client.Proxies()
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		s.Add(proxy)
	}
	return nil
}

// NewProxyStore returns a store of proxies.
// Proxies are stored by path even though proxies are named sans forward slashes.
func NewProxyStore(root url.URL, sep string, client *toxy.Client) *ProxyStore {
	return &ProxyStore{
		root:   root,
		sep:    sep,
		tree:   radix.New(),
		client: client,
	}
}
