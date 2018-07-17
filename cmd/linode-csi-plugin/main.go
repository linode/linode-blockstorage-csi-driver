package main

import (
	"flag"
	"log"

	"github.com/displague/csi-linode/driver"
)

func main() {
	var (
		endpoint = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/com.linode.csi.linodebs/csi.sock", "CSI endpoint")
		token    = flag.String("token", "", "Linode access token")
		url      = flag.String("url", "https://api.linode.com/", "Linode API URL")
	)
	flag.Parse()

	drv, err := driver.NewDriver(*endpoint, *token, *url)
	if err != nil {
		log.Fatalln(err)
	}

	if err := drv.Run(); err != nil {
		log.Fatalln(err)
	}
}
