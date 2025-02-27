package tests

import (
	"context"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	iface "github.com/ipfs/boxo/coreiface"
	"github.com/ipfs/boxo/coreiface/options"
	ipns_pb "github.com/ipfs/boxo/ipns/pb"
)

func (tp *TestSuite) TestRouting(t *testing.T) {
	tp.hasApi(t, func(api iface.CoreAPI) error {
		if api.Routing() == nil {
			return errAPINotImplemented
		}
		return nil
	})

	t.Run("TestRoutingGet", tp.TestRoutingGet)
	t.Run("TestRoutingPut", tp.TestRoutingPut)
	t.Run("TestRoutingPutOffline", tp.TestRoutingPutOffline)
}

func (tp *TestSuite) testRoutingPublishKey(t *testing.T, ctx context.Context, api iface.CoreAPI, opts ...options.NamePublishOption) iface.IpnsEntry {
	p, err := addTestObject(ctx, api)
	if err != nil {
		t.Fatal(err)
	}

	entry, err := api.Name().Publish(ctx, p, opts...)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)
	return entry
}

func (tp *TestSuite) TestRoutingGet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	apis, err := tp.MakeAPISwarm(t, ctx, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Node 1: publishes an IPNS name
	ipnsEntry := tp.testRoutingPublishKey(t, ctx, apis[0])

	// Node 2: retrieves the best value for the IPNS name.
	data, err := apis[1].Routing().Get(ctx, "/ipns/"+ipnsEntry.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Checks if values match.
	var entry ipns_pb.IpnsEntry
	err = proto.Unmarshal(data, &entry)
	if err != nil {
		t.Fatal(err)
	}

	if string(entry.GetValue()) != ipnsEntry.Value().String() {
		t.Fatalf("routing key has wrong value, expected %s, got %s", ipnsEntry.Value().String(), string(entry.GetValue()))
	}
}

func (tp *TestSuite) TestRoutingPut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	apis, err := tp.MakeAPISwarm(t, ctx, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Create and publish IPNS entry.
	ipnsEntry := tp.testRoutingPublishKey(t, ctx, apis[0])

	// Get valid routing value.
	data, err := apis[0].Routing().Get(ctx, "/ipns/"+ipnsEntry.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Put routing value.
	err = apis[1].Routing().Put(ctx, "/ipns/"+ipnsEntry.Name(), data)
	if err != nil {
		t.Fatal(err)
	}
}

func (tp *TestSuite) TestRoutingPutOffline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// init a swarm & publish an IPNS entry to get a valid payload
	apis, err := tp.MakeAPISwarm(t, ctx, 2)
	if err != nil {
		t.Fatal(err)
	}

	ipnsEntry := tp.testRoutingPublishKey(t, ctx, apis[0], options.Name.AllowOffline(true))
	data, err := apis[0].Routing().Get(ctx, "/ipns/"+ipnsEntry.Name())
	if err != nil {
		t.Fatal(err)
	}

	// init our offline node and try to put the payload
	api, err := tp.makeAPIWithIdentityAndOffline(t, ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = api.Routing().Put(ctx, "/ipns/"+ipnsEntry.Name(), data)
	if err == nil {
		t.Fatal("this operation should fail because we are offline")
	}

	err = api.Routing().Put(ctx, "/ipns/"+ipnsEntry.Name(), data, options.Put.AllowOffline(true))
	if err != nil {
		t.Fatal(err)
	}
}
