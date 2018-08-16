package options

import (
	"os"

	"github.com/spf13/pflag"
)

// Config Linode Client Config
type Config struct {
	Endpoint string
	Token    string
	URL      string
	Region   string
	NodeName string
}

// NewConfig Create New Config
func NewConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		Endpoint: "unix:///var/lib/kubelet/plugins/com.linode.csi.linodebs/csi.sock",
		URL:      "https://api.linode.com/v4",
		Token:    "",
		Region:   "",
		NodeName: hostname,
	}
}

// AddFlags Add Flags
func (c *Config) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Endpoint, "endpoint", c.Endpoint, "CSI endpoint")
	fs.StringVar(&c.Token, "token", c.Token, "Linode API Token")
	fs.StringVar(&c.URL, "url", c.URL, "Linode API URL")
	fs.StringVar(&c.Region, "region", c.Region, "Linode Region")
	fs.StringVar(&c.NodeName, "node", c.NodeName, "Linode Hostname")
}
