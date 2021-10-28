package gossip

import (
	"context"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/hashicorp/memberlist"
	"github.com/samueltorres/meshmesh/internal/state"
)

type ServerOptions struct {
	ClusterName         string
	GossipNodeName      string
	GossipAdvertiseAddr string
	GossipAdvertisePort int
	GossipBindAddr      string
	GossipPort          int
	GossipJoinAddresses []string
}

type Server struct {
	options   ServerOptions
	logger    *zap.Logger
	meshState *state.ClusterMeshState
	clientSet *kubernetes.Clientset
}

func NewServer(opts ServerOptions, logger *zap.Logger, meshState *state.ClusterMeshState, clientSet *kubernetes.Clientset) *Server {
	return &Server{
		options:   opts,
		logger:    logger,
		meshState: meshState,
		clientSet: clientSet,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("starting mesh server")

	eventDelegate := NewEventDelegate(s.logger, s.options.ClusterName, s.meshState)
	delegate := NewDelegate(s.logger, s.meshState)

	conf := memberlist.DefaultWANConfig()
	conf.Name = s.options.GossipNodeName
	conf.BindPort = s.options.GossipPort
	conf.BindAddr = s.options.GossipBindAddr
	conf.AdvertisePort = s.options.GossipAdvertisePort
	conf.AdvertiseAddr = s.options.GossipAdvertiseAddr
	conf.Delegate = delegate
	conf.Events = eventDelegate

	// Get ClusterMeshAPIServer LoadBalancer IP
	clusterMeshAPIServerSvc, err := s.clientSet.
		CoreV1().
		Services("kube-system").
		Get(context.Background(), "clustermesh-apiserver", metav1.GetOptions{})
	if err != nil {
		s.logger.Error("getting kubernetes service", zap.Error(err), zap.String("service", "clustermesh-apiserver"))
		return err
	}

	// Get ClusterMeshAPIServer CA
	clusterMeshAPIServerClientCertSecret, err := s.clientSet.
		CoreV1().
		Secrets("kube-system").
		Get(context.Background(), "clustermesh-apiserver-client-cert", metav1.GetOptions{})
	if err != nil {
		s.logger.Error("getting kubernetes secret", zap.Error(err), zap.String("secret", "clustermesh-apiserver-client-cert"))
		return err
	}

	// Get ClusterMeshAPIServer CA
	delegate.nodeMeta.ClusterName = s.options.ClusterName

	s.meshState.Lock()
	s.meshState.AddOrUpdate(&state.ClusterMeshCluster{
		ClusterName:                 s.options.ClusterName,
		ClusterMeshAPIServerIP:      clusterMeshAPIServerSvc.Spec.LoadBalancerIP,
		ClusterMeshAPIServerTLSCA:   string(clusterMeshAPIServerClientCertSecret.Data["ca.crt"]),
		ClusterMeshAPIServerTLSCert: string(clusterMeshAPIServerClientCertSecret.Data["tls.crt"]),
		ClusterMeshAPIServerTLSKey:  string(clusterMeshAPIServerClientCertSecret.Data["tls.key"]),
	})
	s.meshState.Unlock()

	ml, err := memberlist.Create(conf)
	if err != nil {
		return err
	}

	if len(s.options.GossipJoinAddresses) > 0 {
		_, err := ml.Join(s.options.GossipJoinAddresses)
		if err != nil {
			s.logger.Error("error joining addresses", zap.Error(err))
			return err
		}
	}

	s.logger.Info("waiting for finalize memberlist")
	<-ctx.Done()
	s.logger.Info("leaving memberlist")
	return ml.Leave(10 * time.Second)
}
