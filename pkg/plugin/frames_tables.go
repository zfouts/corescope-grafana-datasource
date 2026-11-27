package plugin

import (
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/zfouts/corescope-grafana-datasource/pkg/corescope"
)

// timePtr parses an upstream timestamp; nil or unparseable → nil.
func timePtr(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, ok := corescope.ParseTime(*s)
	if !ok {
		return nil
	}
	return &t
}

func intPtr64(v *int) *int64 {
	if v == nil {
		return nil
	}
	x := int64(*v)
	return &x
}

// NodesFrame is the "nodes" table in upstream row order.
func NodesFrame(l *corescope.NodeList) *data.Frame {
	n := len(l.Nodes)
	pk, name, role := make([]string, n), make([]string, n), make([]string, n)
	heard, first := make([]*time.Time, n), make([]*time.Time, n)
	lat, lon := make([]*float64, n), make([]*float64, n)
	adv, relay := make([]int64, n), make([]int64, n)
	batt, hash := make([]*int64, n), make([]*int64, n)
	score := make([]float64, n)
	for i := range l.Nodes {
		nd := &l.Nodes[i]
		pk[i], name[i], role[i] = SafeString(nd.PublicKey), SafeString(nd.Name), SafeString(nd.Role)
		if nd.LastHeard != nil {
			heard[i] = timePtr(nd.LastHeard)
		}
		if heard[i] == nil {
			heard[i] = timePtr(nd.LastSeen)
		}
		first[i] = timePtr(nd.FirstSeen)
		lat[i], lon[i] = nd.Lat, nd.Lon
		adv[i], relay[i] = int64(nd.AdvertCount), int64(nd.RelayCount24h)
		batt[i], hash[i] = intPtr64(nd.BatteryMv), intPtr64(nd.HashSize)
		score[i] = nd.UsefulnessScore
	}
	return tableMeta(data.NewFrame(string(QueryNodes),
		data.NewField("publicKey", nil, pk),
		data.NewField("name", nil, name),
		data.NewField("role", nil, role),
		data.NewField("lastHeard", nil, heard),
		data.NewField("firstSeen", nil, first),
		data.NewField("lat", nil, lat),
		data.NewField("lon", nil, lon),
		data.NewField("advertCount", nil, adv),
		data.NewField("relayCount24h", nil, relay),
		data.NewField("batteryMv", nil, batt),
		data.NewField("hashSize", nil, hash),
		data.NewField("usefulnessScore", nil, score),
	))
}

// ObserversFrame is the "observers" table in upstream row order.
func ObserversFrame(l *corescope.ObserverList) *data.Frame {
	n := len(l.Observers)
	id, name, iata := make([]string, n), make([]string, n), make([]string, n)
	cv, radio, role := make([]string, n), make([]string, n), make([]string, n)
	seen, first := make([]*time.Time, n), make([]*time.Time, n)
	pc, ph := make([]int64, n), make([]int64, n)
	batt := make([]*int64, n)
	noise, lat, lon := make([]*float64, n), make([]*float64, n), make([]*float64, n)
	for i := range l.Observers {
		o := &l.Observers[i]
		id[i], name[i], iata[i] = SafeString(o.ID), SafeStringPtr(o.Name), SafeStringPtr(o.IATA)
		cv[i], radio[i], role[i] = SafeStringPtr(o.ClientVersion), SafeStringPtr(o.Radio), SafeStringPtr(o.NodeRole)
		seen[i], first[i] = timePtr(o.LastSeen), timePtr(o.FirstSeen)
		pc[i], ph[i] = int64(o.PacketCount), int64(o.PacketsLastHour)
		batt[i] = intPtr64(o.BatteryMv)
		noise[i], lat[i], lon[i] = o.NoiseFloor, o.Lat, o.Lon
	}
	return tableMeta(data.NewFrame(string(QueryObservers),
		data.NewField("id", nil, id),
		data.NewField("name", nil, name),
		data.NewField("iata", nil, iata),
		data.NewField("clientVersion", nil, cv),
		data.NewField("radio", nil, radio),
		data.NewField("nodeRole", nil, role),
		data.NewField("lastSeen", nil, seen),
		data.NewField("firstSeen", nil, first),
		data.NewField("packetCount", nil, pc),
		data.NewField("packetsLastHour", nil, ph),
		data.NewField("batteryMv", nil, batt),
		data.NewField("noiseFloor", nil, noise),
		data.NewField("lat", nil, lat),
		data.NewField("lon", nil, lon),
	))
}
