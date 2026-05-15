// Package discovery handles peer discovery via DHT routing and mDNS.
package discovery

import (
	"context"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"go.uber.org/zap"
)

const rendezvousNS = "/indra/1.0.0"

// Discovery manages peer discovery for the node.
type Discovery struct {
	host host.Host
	dht  *dht.IpfsDHT
	log  *zap.Logger
}

// New creates a Discovery instance and starts mDNS and DHT-based discovery.
func New(ctx context.Context, h host.Host, kadDHT *dht.IpfsDHT, log *zap.Logger) *Discovery {
	d := &Discovery{host: h, dht: kadDHT, log: log}

	// mDNS — zero-config LAN discovery.
	svc := mdns.NewMdnsService(h, rendezvousNS, &mdnsNotifee{h: h, log: log})
	if err := svc.Start(); err != nil {
		log.Warn("mDNS start failed", zap.Error(err))
	}

	// DHT routing discovery — advertise and find peers over the internet.
	rd := drouting.NewRoutingDiscovery(kadDHT)
	dutil.Advertise(ctx, rd, rendezvousNS)
	go d.findPeers(ctx, rd)

	return d
}

func (d *Discovery) findPeers(ctx context.Context, rd *drouting.RoutingDiscovery) {
	peerChan, err := rd.FindPeers(ctx, rendezvousNS)
	if err != nil {
		d.log.Warn("DHT FindPeers failed", zap.Error(err))
		return
	}
	for pi := range peerChan {
		if pi.ID == d.host.ID() {
			continue
		}
		if err := d.host.Connect(ctx, pi); err != nil {
			d.log.Debug("connect to discovered peer failed",
				zap.String("peer", pi.ID.String()),
				zap.Error(err),
			)
		} else {
			d.log.Info("connected to discovered peer", zap.String("peer", pi.ID.String()))
		}
	}
}

// mdnsNotifee handles mDNS peer notifications.
type mdnsNotifee struct {
	h   host.Host
	log *zap.Logger
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.h.ID() {
		return
	}
	n.log.Info("mDNS: found peer", zap.String("peer", pi.ID.String()))
	ctx := context.Background()
	if err := n.h.Connect(ctx, pi); err != nil {
		n.log.Debug("mDNS connect failed", zap.String("peer", pi.ID.String()), zap.Error(err))
	}
}
