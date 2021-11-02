package cilium

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/samueltorres/meshmesh/internal/state"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	caSuffix   = ".etcd-client-ca.crt"
	keySuffix  = ".etcd-client.key"
	certSuffix = ".etcd-client.crt"
)

type ConfigurationUpdater struct {
	logger           *zap.Logger
	meshState        *state.ClusterMeshState
	kubernetesClient *kubernetes.Clientset
}

func NewConfigurationUpdater(logger *zap.Logger, meshState *state.ClusterMeshState, kubernetesClient *kubernetes.Clientset) *ConfigurationUpdater {
	return &ConfigurationUpdater{
		logger:           logger,
		meshState:        meshState,
		kubernetesClient: kubernetesClient,
	}
}

func (c *ConfigurationUpdater) Start(ctx context.Context) error {
	c.logger.Info("running config updater")

	updateCh := c.meshState.UpdateChannel()

	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			c.Update()
		case <-updateCh:
			c.Update()
		case <-ctx.Done():
			c.logger.Info("stopping config updater")
			return nil
		}
	}
}

func (c *ConfigurationUpdater) Update() {
	c.logger.Info("running configuration update loop")
	c.UpdateCiliumClusterMeshSecret()
	c.UpdateCiliumDaemonSetHostAliases()
}

func (c *ConfigurationUpdater) UpdateCiliumClusterMeshSecret() {
	// get secret
	_, err := c.kubernetesClient.CoreV1().Secrets("kube-system").Get(context.Background(), "cilium-clustermesh", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		c.logger.Info("secret not found, creating one", zap.String("secret", "cilium-clustermesh"))
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-clustermesh",
				Namespace: "kube-system",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}

		_, err := c.kubernetesClient.CoreV1().Secrets("kube-system").Create(context.Background(), s, metav1.CreateOptions{})
		if err != nil {
			c.logger.Error("error creating secret secret", zap.Error(err))
			return
		}
	}

	pool := c.meshState.GetAll()
	secretData := []string{}

	// gather all data for the secret
	for _, cluster := range pool {
		secretData = append(
			secretData,
			fmt.Sprintf("\"%s\": \"%s\"",
				cluster.ClusterName,
				base64.StdEncoding.EncodeToString([]byte(generateEtcdInformation(cluster.ClusterName, 2379)))))

		secretData = append(
			secretData,
			fmt.Sprintf("\"%s%s\":\"%s\"",
				cluster.ClusterName,
				caSuffix,
				base64.StdEncoding.EncodeToString([]byte(cluster.ClusterMeshAPIServerTLSCA))))

		secretData = append(
			secretData,
			fmt.Sprintf("\"%s%s\":\"%s\"",
				cluster.ClusterName,
				keySuffix,
				base64.StdEncoding.EncodeToString([]byte(cluster.ClusterMeshAPIServerTLSKey))))

		secretData = append(
			secretData,
			fmt.Sprintf("\"%s%s\":\"%s\"",
				cluster.ClusterName,
				certSuffix,
				base64.StdEncoding.EncodeToString([]byte(cluster.ClusterMeshAPIServerTLSCert))))
	}
	sort.Strings(secretData)

	patch := []byte(`{"data":{` + strings.Join(secretData, ",") + `}}`)
	_, err = c.kubernetesClient.
		CoreV1().
		Secrets("kube-system").
		Patch(context.Background(), "cilium-clustermesh", types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		c.logger.Error("error updating secret", zap.Error(err), zap.String("secret", "cilium-clustermesh"))
	}
}

func (c *ConfigurationUpdater) UpdateCiliumDaemonSetHostAliases() {
	pool := c.meshState.GetAll()

	var aliases []string
	for _, cluster := range pool {
		aliases = append(aliases, `{"ip":"`+cluster.ClusterMeshAPIServerIP+`", "hostnames":["`+cluster.ClusterName+`.mesh.cilium.io"]}`)
	}
	sort.Strings(aliases)

	patch := []byte(`{"spec":{"template":{"spec":{"hostAliases":[` + strings.Join(aliases, ",") + `]}}}}`)
	_, err := c.kubernetesClient.
		AppsV1().
		DaemonSets("kube-system").
		Patch(context.Background(), "cilium", types.MergePatchType, patch, metav1.PatchOptions{})

	if err != nil {
		c.logger.Error("error updating daemonset", zap.Error(err), zap.String("daemonset", "cilium"))
		return
	}
}

func generateEtcdInformation(clusterName string, servicePort int) string {
	cfg := "endpoints:\n"
	cfg += "- https://" + clusterName + ".mesh.cilium.io:" + fmt.Sprintf("%d", servicePort) + "\n"
	cfg += "trusted-ca-file: /var/lib/cilium/clustermesh/" + clusterName + caSuffix + "\n"
	cfg += "key-file: /var/lib/cilium/clustermesh/" + clusterName + keySuffix + "\n"
	cfg += "cert-file: /var/lib/cilium/clustermesh/" + clusterName + certSuffix + "\n"

	return cfg
}
