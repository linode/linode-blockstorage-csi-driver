package options

import (
	"os"

	"github.com/spf13/pflag"
)

type Config struct {
	Endpoint string
	Token    string
	Url      string
	Region   string
	NodeName string
}

func NewConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		Endpoint: "unix:///var/lib/kubelet/plugins/com.linode.csi.linodebs/csi.sock",
		Url:      "https://api.linode.com/",
		Token:    "",
		Region:   "",
		NodeName: hostname,
	}
}

func (c *Config) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Endpoint, "endpoint", c.Endpoint, "CSI endpoint")
	fs.StringVar(&c.Token, "token", c.Token, "Linode access token")
	fs.StringVar(&c.Url, "url", c.Url, "Linode API URL")
	fs.StringVar(&c.Region, "region", c.Region, "Linode Region")
	fs.StringVar(&c.NodeName, "node", c.NodeName, "Linode Hostname")
}
