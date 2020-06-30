package discovery

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/saveio/carrier/dht"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/peer"
	"github.com/saveio/themis/common/log"
)

type lookupResult struct {
	peer      peer.ID
	responses []*protobuf.ID
}

func queryPeerByIDExt(net *network.Network, peerID peer.ID, targetID peer.ID, responses chan *lookupResult) {
	component, _ := net.Component(ComponentID)
	router := component.(*Component)

	client, err := net.Client(peerID.Address, peerID.PublicKeyHex())

	if err != nil {
		log.Debugf("queryPeerByIDExt create Client for %s fail %s", peerID.Address, err)
		responses <- &lookupResult{peer: peerID, responses: []*protobuf.ID{}}
		return
	}

	targetProtoID := protobuf.ID(targetID)

	msg := &protobuf.LookupNodeRequest{Target: &targetProtoID}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := client.Request(ctx, msg, 3*time.Second)

	if err != nil {
		log.Debugf("queryPeerByIDExt peer %s time out", peerID.Address)
		responses <- &lookupResult{peer: peerID, responses: []*protobuf.ID{}}
		return
	}

	if response, ok := response.(*protobuf.LookupNodeResponse); ok {
		validPeers := []*protobuf.ID{}
		for _, id := range response.Peers {
			log.Debugf("queryPeerByIDExt get peer Address %s", id.Address)

			//ensure peers are online!
			peerId := peer.ID(*id)
			if !net.ClientExist(peerId.PublicKeyHex()) {
				clientID := net.Bootstrap([]string{id.Address})
				if len(clientID) != 0 {
					validPeers = append(validPeers, id)
					router.Routes.Update(peerId)
				}
			} else {
				validPeers = append(validPeers, id)
			}

		}

		responses <- &lookupResult{peer: peerID, responses: validPeers}
	} else {
		responses <- &lookupResult{peer: peerID, responses: []*protobuf.ID{}}
	}
}

//always remember closest peer
func (lookup *lookupBucket) performLookupClosest(net *network.Network, targetID peer.ID, alpha int, visited *sync.Map) (results []peer.ID) {
	responses := make(chan *lookupResult)
	component, _ := net.Component(ComponentID)
	router := component.(*Component)
	known := new(sync.Map)

	// Go through every peer in the entire queue and queue up what peers believe
	// is closest to a target ID.
	for ; lookup.pending < alpha && len(lookup.queue) > 0; lookup.pending++ {
		go queryPeerByIDExt(net, lookup.queue[0], targetID, responses)
		known.LoadOrStore(lookup.queue[0].PublicKeyHex(), struct{}{})
		lookup.queue = lookup.queue[1:]
	}

	// Asynchronous breadth-first search.
	for lookup.pending > 0 {
		result := <-responses
		response := result.responses
		lookup.pending--

		//remove failed peer
		if len(response) == 0 {
			router.Routes.RemovePeer(result.peer)
		}

		//get top k closest !
		for _, id := range response {
			peerID := peer.ID(*id)

			router.Routes.Update(peerID)
			if _, seen := known.Load(peerID.PublicKeyHex()); !seen {
				results = append(results, peerID)
				lookup.queue = append(lookup.queue, peerID)
			}
		}

		//get top k
		sort.Slice(lookup.queue, func(i, j int) bool {
			left := results[i].XorID(targetID)
			right := results[j].XorID(targetID)
			return left.Less(right)
		})

		if len(lookup.queue) > dht.BucketSize {
			lookup.queue = lookup.queue[:dht.BucketSize]
		}

		//schedule new search
		i := 0
		for ; lookup.pending < alpha && i < len(lookup.queue); i++ {
			if _, seen := visited.LoadOrStore(lookup.queue[i].PublicKeyHex(), struct{}{}); !seen {
				go queryPeerByIDExt(net, lookup.queue[i], targetID, responses)
				lookup.pending++
			}
		}
	}

	return
}

func queryPeerByID(net *network.Network, peerID peer.ID, targetID peer.ID, responses chan []*protobuf.ID) {
	client, err := net.Client(peerID.Address, fmt.Sprintf("%s", peerID.Id))
	if err != nil {
		responses <- []*protobuf.ID{}
		return
	}

	targetProtoID := protobuf.ID(targetID)

	msg := &protobuf.LookupNodeRequest{Target: &targetProtoID}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := client.Request(ctx, msg, 3*time.Second)

	if err != nil {
		responses <- []*protobuf.ID{}
		return
	}

	if response, ok := response.(*protobuf.LookupNodeResponse); ok {
		responses <- response.Peers
	} else {
		responses <- []*protobuf.ID{}
	}
}

type lookupBucket struct {
	pending int
	queue   []peer.ID
}

func (lookup *lookupBucket) performLookup(net *network.Network, targetID peer.ID, alpha int, visited *sync.Map) (results []peer.ID) {
	responses := make(chan []*protobuf.ID)

	// Go through every peer in the entire queue and queue up what peers believe
	// is closest to a target ID.

	for ; lookup.pending < alpha && len(lookup.queue) > 0; lookup.pending++ {
		go queryPeerByID(net, lookup.queue[0], targetID, responses)

		results = append(results, lookup.queue[0])
		lookup.queue = lookup.queue[1:]
	}

	// Empty queue.
	lookup.queue = lookup.queue[:0]

	// Asynchronous breadth-first search.
	for lookup.pending > 0 {
		response := <-responses

		lookup.pending--

		// Expand responses containing a peer's belief on the closest peers to target ID.
		for _, id := range response {
			peerID := peer.ID(*id)

			if _, seen := visited.LoadOrStore(peerID.PublicKeyHex(), struct{}{}); !seen {
				// Append new peer to be queued by the routing table.
				results = append(results, peerID)
				lookup.queue = append(lookup.queue, peerID)
			}
		}

		// Queue and request for #ALPHA closest peers to target ID from expanded results.
		for ; lookup.pending < alpha && len(lookup.queue) > 0; lookup.pending++ {
			go queryPeerByID(net, lookup.queue[0], targetID, responses)
			lookup.queue = lookup.queue[1:]
		}

		// Empty queue.
		lookup.queue = lookup.queue[:0]
	}

	return
}

// FindNode queries all peers this current node acknowledges for the closest peers
// to a specified target ID.
//
// All lookups are done under a number of disjoint lookups in parallel.
//
// Queries at most #ALPHA nodes at a time per lookup, and returns all peer IDs closest to a target peer ID.
func FindNode(net *network.Network, targetID peer.ID, alpha int, disjointPaths int) (results []peer.ID) {
	component, exists := net.Component(ComponentID)

	// Discovery Component was not registered. Fail.
	if !exists {
		return
	}

	visited := new(sync.Map)

	var lookups []*lookupBucket

	// Start searching for target from #ALPHA peers closest to target by queuing
	// them up and marking them as visited.
	for i, peerID := range component.(*Component).Routes.FindClosestPeers(targetID, alpha) {
		visited.Store(peerID.PublicKeyHex(), struct{}{})

		//don't call self
		if peerID.Address == net.ID.Address {
			continue
		}

		if len(lookups) < disjointPaths {
			lookups = append(lookups, new(lookupBucket))
		}

		lookup := lookups[i%disjointPaths]
		lookup.queue = append(lookup.queue, peerID)

		results = append(results, peerID)
	}
	//set self as visited!
	visited.Store(net.ID.PublicKeyHex(), struct{}{})

	wait, mutex := &sync.WaitGroup{}, &sync.Mutex{}

	for _, lookup := range lookups {
		wait.Add(1)
		go func(lookup *lookupBucket) {
			mutex.Lock()
			results = append(results, lookup.performLookupClosest(net, targetID, alpha, visited)...)
			mutex.Unlock()

			wait.Done()
		}(lookup)
	}

	// Wait until all #D parallel lookups have been completed.
	wait.Wait()

	// Sort resulting peers by XOR distance.
	sort.Slice(results, func(i, j int) bool {
		left := results[i].Xor(targetID)
		right := results[j].Xor(targetID)
		return left.Less(right)
	})

	// Cut off list of results to only have the routing table focus on the
	// #dht.BucketSize closest peers to the current node.
	if len(results) > dht.BucketSize {
		results = results[:dht.BucketSize]
	}

	return
}

//suggest random peers
func SuggestPeers(net *network.Network, count int) (results []peer.ID) {
	component, exists := net.Component(ComponentID)

	// Discovery Component was not registered. Fail.
	if !exists {
		return
	}

	results = component.(*Component).Routes.GetRandomPeers(count)

	return
}

func UpdateSelf(net *network.Network, newId peer.ID) {
	component, exists := net.Component(ComponentID)

	// Discovery Component was not registered. Fail.
	if !exists {
		return
	}

	oldId := component.(*Component).Routes.Self()
	if oldId.Equals(newId) {
		component.(*Component).Routes.Update(newId)
	}

	return
}
