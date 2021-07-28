package loadbalance

import (
	"fmt"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	namingpb "github.com/polarismesh/polaris-go/pkg/model/pb/v1"
	"github.com/polarismesh/polaris-go/pkg/network"
	"github.com/polarismesh/polaris-go/test/mock"
	"github.com/polarismesh/polaris-go/test/util"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gopkg.in/check.v1"
	"log"
	"math"
	"net"
	"os"
)

//LBTestingSuite 消费者API测试套
type InnerServiceLBTestingSuite struct {
	grpcServer        *grpc.Server
	grpcListener      net.Listener
	idInstanceWeights map[instanceKey]int
	idInstanceCalls   map[instanceKey]int
	mockServer        mock.NamingServer

	monitorToken string
}

//设置模拟桩服务器
func (t *InnerServiceLBTestingSuite) SetUpSuite(c *check.C) {
	grpcOptions := make([]grpc.ServerOption, 0)
	maxStreams := 100000
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(uint32(maxStreams)))

	// get the grpc server wired up
	grpc.EnableTracing = true

	ipAddr := lbIPAddr
	shopPort := lbPort
	var err error
	t.grpcServer = grpc.NewServer(grpcOptions...)
	t.mockServer = mock.NewNamingServer()
	token := t.mockServer.RegisterServerService(config.ServerDiscoverService)
	t.mockServer.RegisterServerInstance(ipAddr, shopPort, config.ServerDiscoverService, token, true)

	namingpb.RegisterPolarisGRPCServer(t.grpcServer, t.mockServer)
	t.grpcListener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", ipAddr, shopPort))
	if nil != err {
		log.Fatal(fmt.Sprintf("error listening appserver %v", err))
	}
	log.Printf("appserver listening on %s:%d\n", ipAddr, shopPort)
	go func() {
		t.grpcServer.Serve(t.grpcListener)
	}()

	t.monitorToken = t.mockServer.RegisterServerService(config.ServerMonitorService)
}

//SetUpSuite 结束测试套程序
func (t *InnerServiceLBTestingSuite) TearDownSuite(c *check.C) {
	t.grpcServer.Stop()
	if util.DirExist(util.BackupDir) {
		os.RemoveAll(util.BackupDir)
	}
}

func (t *InnerServiceLBTestingSuite) TestConnManger(c *check.C) {
	service := &namingpb.Service{
		Name:      &wrappers.StringValue{Value: config.ServerMonitorService},
		Namespace: &wrappers.StringValue{Value: config.ServerNamespace},
		Token:     &wrappers.StringValue{Value: uuid.New().String()},
	}
	var Instances []*namingpb.Instance

	Instances = append(Instances, &namingpb.Instance{
		Id:        &wrappers.StringValue{Value: uuid.New().String()},
		Service:   &wrappers.StringValue{Value: config.ServerMonitorService},
		Namespace: &wrappers.StringValue{Value: config.ServerNamespace},
		Host:      &wrappers.StringValue{Value: "127.0.0.1"},
		Port:      &wrappers.UInt32Value{Value: uint32(10030 + 1)},
		Weight:    &wrappers.UInt32Value{Value: uint32(100)},
		Metadata: map[string]string{
			"protocol": "grpc",
		},
	})
	Instances = append(Instances, &namingpb.Instance{
		Id:        &wrappers.StringValue{Value: uuid.New().String()},
		Service:   &wrappers.StringValue{Value: config.ServerMonitorService},
		Namespace: &wrappers.StringValue{Value: config.ServerNamespace},
		Host:      &wrappers.StringValue{Value: "127.0.0.1"},
		Port:      &wrappers.UInt32Value{Value: uint32(10030 + 2)},
		Weight:    &wrappers.UInt32Value{Value: uint32(300)},
		Metadata: map[string]string{
			"protocol": "grpc",
		},
	})
	Instances = append(Instances, &namingpb.Instance{
		Id:        &wrappers.StringValue{Value: uuid.New().String()},
		Service:   &wrappers.StringValue{Value: config.ServerMonitorService},
		Namespace: &wrappers.StringValue{Value: config.ServerNamespace},
		Host:      &wrappers.StringValue{Value: "127.0.0.1"},
		Port:      &wrappers.UInt32Value{Value: uint32(10030 + 3)},
		Weight:    &wrappers.UInt32Value{Value: uint32(500)},
		Metadata: map[string]string{
			"protocol": "grpc",
		},
	})
	fmt.Println(Instances)
	t.mockServer.RegisterServiceInstances(service, Instances)
	cfg, err := config.LoadConfigurationByFile("testdata/consumer.yaml")
	consumer, err := api.NewConsumerAPIByConfig(cfg)
	c.Assert(err, check.IsNil)
	defer consumer.Destroy()

	mgr, err := network.NewConnectionManager(cfg, consumer.SDKContext().GetValueContext())
	c.Assert(err, check.IsNil)

	w100Count := 0
	w300Count := 0
	w500Count := 0

	for i := 0; i < 10000; i++ {
		_, inst, err := mgr.GetHashExpectedInstance(config.MonitorCluster, []byte(fmt.Sprintf("%d", i)))
		c.Assert(err, check.IsNil)
		weight := inst.GetWeight()
		switch weight {
		case 100:
			w100Count++
		case 300:
			w300Count++
		case 500:
			w500Count++
		}
	}
	fmt.Println(w100Count, w300Count, w500Count)
	a1 := float64(w300Count) / float64(w100Count)
	a2 := float64(w500Count) / float64(w100Count)
	fmt.Println(a1, a2)
	c.Assert(math.Abs(a1-3) < 0.8, check.Equals, true)
	c.Assert(math.Abs(a2-5) < 0.8, check.Equals, true)
}