package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"./seed"

	"github.com/niftynei/glightning/glightning"
	log "github.com/Sirupsen/logrus"
)

var (
	lightningRpcs []*glightning.Lightning

	listenAddr    = flag.String("listen", "0.0.0.0:53", "Listen address for incoming requests.")
	rootDomain    = flag.String("root-domain", "lseed.bitcoinstats.com", "Root DNS seed domain.")
	pollInterval  = flag.Int("poll-interval", 10, "Time between polls to lightningd for updates")
	lightningDir  = flag.String("lightning-dir", filepath.Join(os.Getenv("HOME"),".lightning"), "The lightning directory.")
	network       = flag.String("network", "bitcoin", "The network to run the seeder on. One of 'bitcoin', 'testnet', or 'all' for both.")
	lightningSock = flag.String("lightning-sock", "lightning-rpc", "Name of the lightning RPC socket")
	debug         = flag.Bool("debug", false, "Be very verbose")
	numResults    = flag.Int("results", 25, "How many results shall we return to a query?")
)

// Expand variables in paths such as $HOME
func expandVariables() error {
	user, err := user.Current()
	if err != nil {
		return err
	}
	*lightningSock = strings.Replace(*lightningSock, "$HOME", user.HomeDir, -1)
	return nil
}

// Regularly polls the lightningd node and updates the local NetworkView.
func poller(lrpc *glightning.Lightning, nview *seed.NetworkView) {
	scrapeGraph := func() {
		nodes, err := lrpc.ListNodes()

		if err != nil {
			log.Errorf("Error trying to get update from lightningd: %v", err)
		} else {
			log.Debugf("Got %d nodes from lightningd", len(nodes))
			for _, n := range nodes {
				if len(n.Addresses) == 0 {
					continue
				}
				nview.AddNode(n)
			}
		}
	}

	scrapeGraph()

	ticker := time.NewTicker(time.Second * time.Duration(*pollInterval))
	for range ticker.C {
		scrapeGraph()
	}
}

// Parse flags and configure subsystems according to flags
func configure() {
	flag.Parse()
	expandVariables()
	if *debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

func NewLightningRpc(lightningDir *string, network string, socketName *string) (*glightning.Lightning) {
	lRpc := glightning.NewLightning()
	lRpc.StartUp(*socketName, filepath.Join(*lightningDir, network))
	return lRpc
}

func GetRealms(network *string) ([]int) {
	if *network == "all" {
		return []int{0, 1}
	} else if *network == "bitcoin" {
		return []int{0}
	} else if *network == "testnet" {
		return []int{1}
	} else {
		return nil
	}
}

// Main entry point for the lightning-seed
func main() {
	configure()
	if *network == "all" {
		lightningRpcs[0] = NewLightningRpc(lightningDir, "bitcoin", lightningSock)
		lightningRpcs[1] = NewLightningRpc(lightningDir, "testnet", lightningSock)
	} else if *network == "bitcoin" || *network == "testnet" {
		lightningRpcs[0] = NewLightningRpc(lightningDir, *network, lightningSock)
	} else {
		fmt.Sprintf("Unsupported network: %s.", *network)
		os.Exit(1)
	}

	realms := GetRealms(network)
	nview := seed.NewNetworkView()
	dnsServer := seed.NewDnsServer(nview, *listenAddr, *rootDomain, realms)

	for _, lRpc := range lightningRpcs {
		go poller(lRpc, nview)
	}

		dnsServer.Serve()
}
