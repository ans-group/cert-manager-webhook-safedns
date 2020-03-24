package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
	certmanagermetav1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	ukfastclient "github.com/ukfast/sdk-go/pkg/client"
	ukfastconnection "github.com/ukfast/sdk-go/pkg/connection"
	"github.com/ukfast/sdk-go/pkg/service/safedns"

	log "log"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&safeDNSProviderSolver{},
	)
}

type safeDNSProviderSolver struct {
	client *kubernetes.Clientset
}

// safeDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer
type safeDNSProviderConfig struct {
	APIKeySecretRef certmanagermetav1.SecretKeySelector `json:"apiKeySecretRef"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource
func (c *safeDNSProviderSolver) Name() string {
	return "safedns"
}

// Present creates a record in SafeDNS for given Challenge Request ch
func (c *safeDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	service, err := c.getSafeDNSService(ch)
	if err != nil {
		return err
	}

	sanitisedRecordZoneName := sanitiseDNSName(ch.ResolvedZone)
	sanitisedRecordName := sanitiseDNSName(ch.ResolvedFQDN)

	req := safedns.CreateRecordRequest{
		Name:    sanitisedRecordName,
		Type:    safedns.RecordTypeTXT.String(),
		Content: getTXTRecordContent(ch.Key),
	}

	log.Printf("Creating record '%s' in zone '%s'", sanitisedRecordName, sanitisedRecordZoneName)
	_, err = service.CreateZoneRecord(sanitisedRecordZoneName, req)
	return err
}

// CleanUp removes a record from SafeDNS for given Challenge Request ch
func (c *safeDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	service, err := c.getSafeDNSService(ch)
	if err != nil {
		return err
	}

	sanitisedRecordZoneName := sanitiseDNSName(ch.ResolvedZone)
	sanitisedRecordName := sanitiseDNSName(ch.ResolvedFQDN)

	params := ukfastconnection.APIRequestParameters{}
	params.WithFilter(ukfastconnection.APIRequestFiltering{
		Property: "name",
		Operator: ukfastconnection.EQOperator,
		Value:    []string{sanitisedRecordName},
	})
	params.WithFilter(ukfastconnection.APIRequestFiltering{
		Property: "type",
		Operator: ukfastconnection.EQOperator,
		Value:    []string{safedns.RecordTypeTXT.String()},
	})
	params.WithFilter(ukfastconnection.APIRequestFiltering{
		Property: "content",
		Operator: ukfastconnection.EQOperator,
		Value:    []string{getTXTRecordContent(ch.Key)},
	})

	log.Printf("Retrieving TXT record '%s' for zone '%s'", sanitisedRecordName, sanitisedRecordZoneName)
	records, err := service.GetZoneRecords(sanitisedRecordZoneName, params)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		return fmt.Errorf("No existing records found for '%s' in zone '%s'", sanitisedRecordName, sanitisedRecordZoneName)
	}

	log.Printf("Deleting zone record '%d' in zone '%s'", records[0].ID, sanitisedRecordZoneName)
	return service.DeleteZoneRecord(sanitisedRecordZoneName, records[0].ID)
}

// Initialize will be called when the webhook first starts
func (c *safeDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = cl
	return nil
}

func (c *safeDNSProviderSolver) getSafeDNSService(ch *v1alpha1.ChallengeRequest) (safedns.SafeDNSService, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return nil, err
	}

	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(cfg.APIKeySecretRef.LocalObjectReference.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	secBytes, ok := sec.Data[cfg.APIKeySecretRef.Key]
	if !ok {
		return nil, fmt.Errorf("Key '%s' not found in secret '%s/%s'", cfg.APIKeySecretRef.Key, ch.ResourceNamespace, cfg.APIKeySecretRef.LocalObjectReference.Name)
	}

	return ukfastclient.NewClient(ukfastconnection.NewAPIKeyCredentialsAPIConnection(string(secBytes))).SafeDNSService(), nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (safeDNSProviderConfig, error) {
	cfg := safeDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

func sanitiseDNSName(name string) string {
	return strings.TrimSuffix(name, ".")
}

func getTXTRecordContent(key string) string {
	return "\"" + key + "\""
}
