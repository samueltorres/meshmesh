package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/oklog/run"
	"github.com/samueltorres/meshmesh/internal/cilium"
	"github.com/samueltorres/meshmesh/internal/gossip"
	"github.com/samueltorres/meshmesh/internal/state"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {

	var outOfClusterConfig bool
	var kubeContext string
	var clusterName string
	var gossipNodeName string
	var gossipAdvertiseAddr string
	var gossipAdvertisePort int
	var gossipPort int
	var gossipBindAddr string
	var gossipJoinAddresses string

	logLevel := zap.LevelFlag("log-level", zap.InfoLevel, "")
	flag.BoolVar(&outOfClusterConfig, "out-of-cluster-config", false, "")
	flag.StringVar(&kubeContext, "context", "", "")
	flag.StringVar(&clusterName, "cluster-name", "", "")
	flag.StringVar(&gossipNodeName, "gossip-node-name", "", "")
	flag.StringVar(&gossipAdvertiseAddr, "gossip-advertise-addr", "", "")
	flag.IntVar(&gossipAdvertisePort, "gossip-advertise-port", 0, "")
	flag.StringVar(&gossipBindAddr, "gossip-bind-addr", "127.0.0.1", "")
	flag.IntVar(&gossipPort, "gossip-port", 0, "")
	flag.StringVar(&gossipJoinAddresses, "gossip-join-servers", "", "")
	flag.Parse()

	loggerCfg := zap.NewProductionConfig()
	loggerCfg.Level.SetLevel(*logLevel)
	logger, err := loggerCfg.Build()
	if err != nil {
		panic(err.Error())
	}

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	var kubernetesConfig *rest.Config
	if outOfClusterConfig {
		var err error
		kubernetesConfig, err = buildConfigFromFlags(kubeContext, *kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		kubernetesConfig, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	kubernetesClient, err := kubernetes.NewForConfig(kubernetesConfig)
	if err != nil {
		panic(err.Error())
	}

	// ClusterMesh State
	state := state.NewClusterMeshState(logger)

	// Cilium config updater
	ciliumCfgUpdater := cilium.NewConfigurationUpdater(logger, state, kubernetesClient)

	// Gossip Server
	gossipServerOpts := gossip.ServerOptions{
		ClusterName:         clusterName,
		GossipNodeName:      gossipNodeName,
		GossipAdvertiseAddr: gossipAdvertiseAddr,
		GossipAdvertisePort: gossipAdvertisePort,
		GossipPort:          gossipPort,
		GossipBindAddr:      gossipBindAddr,
		GossipJoinAddresses: parseAddressList(gossipJoinAddresses),
	}
	gossipServer := gossip.NewServer(gossipServerOpts, logger, state, kubernetesClient)

	ctx, cancel := context.WithCancel(context.Background())
	var g run.Group
	{
		g.Add(func() error {
			return ciliumCfgUpdater.Start(ctx)
		}, func(err error) {
			cancel()
		})
	}
	{
		g.Add(func() error {
			return gossipServer.Start(ctx)
		}, func(err error) {
			cancel()
		})
	}
	{
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			sig := <-c
			cancel()
			logger.Info("received signal", zap.String("signal", sig.String()))
			return nil
		}, func(error) {
		})
	}

	logger.Info("finished", zap.Error(g.Run()))
}

func parseAddressList(addresses string) []string {
	var r []string
	s := strings.Split(addresses, ",")
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func buildConfigFromFlags(context, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}
