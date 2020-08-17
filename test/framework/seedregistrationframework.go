package framework

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	"github.com/onsi/ginkgo"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var seedCreationConfig *SeedCreationConfig

const (
	kubeconfigString = "kubeconfig"
)

// SeedCreationFramework represents the seed test framework that includes
// test functions that can be executed ona specific seed
type SeedCreationFramework struct {
	*GardenerFramework
	TestDescription
	Config *SeedCreationConfig

	SeedClient kubernetes.Interface
	Seed       *gardencorev1beta1.Seed
	Secret     *corev1.Secret
}

// SeedCreationConfig is the configuration for a seed framework that will be filled with user provided data
type SeedCreationConfig struct {
	GardenerConfig        *GardenerConfig
	SeedName              string
	IngressDomain         string
	SecretRefName         string
	SecretRefNamespace    string
	NewSecretName         string
	NewSecretNamespace    string
	BackupSecretName      string
	BackupSecretNamespace string
	BackupSecretProvider  string
	ShootedSeedName       string
	ShootedSeedNamespace  string
	ShootedSeedKubeconfig string
	Provider              string
	ProviderType          string
	NetworkDefServices    string
	NetworkDefPods        string
	BlockCIDRs            string
}

// NewSeedCreationFramework creates a new simple Seed framework
func NewSeedCreationFramework(cfg *SeedCreationConfig) *SeedCreationFramework {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}

	f := &SeedCreationFramework{
		GardenerFramework: NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   NewTestDescription("SEED"),
		Config:            cfg,
	}

	CBeforeEach(func(ctx context.Context) {
		f.CommonFramework.BeforeEach()
		f.GardenerFramework.BeforeEach()
		f.BeforeEach(ctx)
	}, 8*time.Minute)
	CAfterEach(f.AfterEach, 10*time.Minute)
	return f
}

// NewSeedCreationFrameworkFromConfig creates a new seed framework from a seed configuration without registering ginkgo
// specific functions
func NewSeedCreationFrameworkFromConfig(cfg *SeedCreationConfig) (*SeedCreationFramework, error) {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}
	f := &SeedCreationFramework{
		GardenerFramework: NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   NewTestDescription("SEED"),
		Config:            cfg,
	}
	if cfg != nil && gardenerConfig != nil {

	}
	return f, nil
}

// BeforeEach should be called in ginkgo's BeforeEach.
// It sets up the seed framework.
func (f *SeedCreationFramework) BeforeEach(ctx context.Context) {
	f.Config = mergeSeedConfig(f.Config, seedCreationConfig)
	validateSeedConfig(f.Config)
}

// AfterEach should be called in ginkgo's AfterEach.
// Cleans up resources and dumps the shoot state if the test failed
func (f *SeedCreationFramework) AfterEach(ctx context.Context) {
	if ginkgo.CurrentGinkgoTestDescription().Failed {
		f.DumpState(ctx)
	}
}

func validateSeedConfig(cfg *SeedCreationConfig) {
	if cfg == nil {
		ginkgo.Fail("no shoot framework configuration provided")
	}
	if !StringSet(cfg.SeedName) {
		ginkgo.Fail("You should specify a name for the new Seed")
	}
	if !StringSet(cfg.IngressDomain) {
		ginkgo.Fail("You should specify a IngressDomain")
	}
	if !StringSet(cfg.SecretRefName) {
		ginkgo.Fail("You should specify a name for the reference secret which fields will be copied")
	}
	if !StringSet(cfg.SecretRefNamespace) {
		ginkgo.Fail("You should specify a namespace for the reference secret which fields will be copied")
	}
	if !StringSet(cfg.NewSecretName) {
		ginkgo.Fail("You should specify a name for the new secret that will be created")
	}
	if !StringSet(cfg.NewSecretNamespace) {
		ginkgo.Fail("You should specify a namespace for the new secret that will be created")
	}
	if !StringSet(cfg.ShootedSeedName) && !StringSet(cfg.ShootedSeedNamespace) {
		ginkgo.Fail("You should specify Shooted Seed name and namespace to test against")
	}
	if !StringSet(cfg.Provider) && !StringSet(cfg.ProviderType) {
		ginkgo.Fail("You should specify a Provider and Provider type")
	}
}

func mergeSeedConfig(base, overwrite *SeedCreationConfig) *SeedCreationConfig {
	if base == nil {
		return overwrite
	}
	if overwrite == nil {
		return base
	}
	if overwrite.GardenerConfig != nil {
		base.GardenerConfig = overwrite.GardenerConfig
	}
	if StringSet(overwrite.SeedName) {
		base.SeedName = overwrite.SeedName
	}
	if StringSet(overwrite.IngressDomain) {
		base.IngressDomain = overwrite.IngressDomain
	}
	if StringSet(overwrite.SecretRefName) {
		base.SecretRefName = overwrite.SecretRefName
	}
	if StringSet(overwrite.SecretRefNamespace) {
		base.SecretRefNamespace = overwrite.SecretRefNamespace
	}
	if StringSet(overwrite.NewSecretName) {
		base.NewSecretName = overwrite.NewSecretName
	}
	if StringSet(overwrite.NewSecretNamespace) {
		base.NewSecretNamespace = overwrite.NewSecretNamespace
	}
	if StringSet(overwrite.BackupSecretName) {
		base.BackupSecretName = overwrite.BackupSecretName
	}
	if StringSet(overwrite.BackupSecretNamespace) {
		base.BackupSecretNamespace = overwrite.BackupSecretNamespace
	}
	if StringSet(overwrite.BackupSecretProvider) {
		base.BackupSecretProvider = overwrite.BackupSecretProvider
	}
	if StringSet(overwrite.ShootedSeedName) {
		base.ShootedSeedName = overwrite.ShootedSeedName
	}
	if StringSet(overwrite.ShootedSeedNamespace) {
		base.ShootedSeedNamespace = overwrite.ShootedSeedNamespace
	}
	if StringSet(overwrite.ShootedSeedKubeconfig) {
		base.ShootedSeedKubeconfig = overwrite.ShootedSeedKubeconfig
	}
	if StringSet(overwrite.Provider) {
		base.Provider = overwrite.Provider
	}
	if StringSet(overwrite.ProviderType) {
		base.ProviderType = overwrite.ProviderType
	}
	if StringSet(overwrite.BlockCIDRs) {
		base.BlockCIDRs = overwrite.BlockCIDRs
	}
	if StringSet(overwrite.NetworkDefServices) {
		base.NetworkDefServices = overwrite.NetworkDefServices
	}
	if StringSet(overwrite.NetworkDefPods) {
		base.NetworkDefPods = overwrite.NetworkDefPods
	}
	return base
}

// RegisterSeedCreationFrameworkFlags adds all flags that are needed to configure a shoot framework to the provided flagset.
func RegisterSeedCreationFrameworkFlags() *SeedCreationConfig {
	_ = RegisterGardenerFrameworkFlags()

	newCfg := &SeedCreationConfig{}

	flag.StringVar(&newCfg.SeedName, "seed-name", "", "name of the seed")
	flag.StringVar(&newCfg.IngressDomain, "ingress-domain", "", "ingress domain")
	//Secrets
	flag.StringVar(&newCfg.SecretRefName, "secret-ref-name", "", "name of the secret reference")
	flag.StringVar(&newCfg.SecretRefNamespace, "secret-ref-namespace", "", "namespace of the secret reference")
	flag.StringVar(&newCfg.NewSecretName, "new-secret-name", "", "name of the new secret reference")
	flag.StringVar(&newCfg.NewSecretNamespace, "new-secret-namespace", "", "namespace of the new secret reference")
	flag.StringVar(&newCfg.BackupSecretName, "backup-secret-name", "", "name of the backup secret reference")
	flag.StringVar(&newCfg.BackupSecretNamespace, "backup-secret-namespace", "", "namespace of the backup secret reference")
	flag.StringVar(&newCfg.BackupSecretProvider, "backup-secret-provider", "", "namespace of the backup secret reference")
	//Shooted seed reference
	flag.StringVar(&newCfg.ShootedSeedName, "shooted-seed-name", "", "name of the seed")
	flag.StringVar(&newCfg.ShootedSeedNamespace, "shooted-seed-namespace", "", "name of the seed")
	flag.StringVar(&newCfg.ShootedSeedKubeconfig, "seed-kubecfg", "", "kubeconfig of the shooted seed")
	//Provider
	flag.StringVar(&newCfg.ProviderType, "provider-type", "", "provider type e.g. aws, az, gcp")
	flag.StringVar(&newCfg.Provider, "provider-region", "", "provider region e.g. eu-west-1")
	//Network
	flag.StringVar(&newCfg.NetworkDefServices, "network-default-services", "", "Value for Seed.Spec.Networks.ShootDefaults.Services")
	flag.StringVar(&newCfg.NetworkDefPods, "network-default-pods", "", "Value for Seed.Spec.Networks.ShootDefaults.Pods")
	flag.StringVar(&newCfg.BlockCIDRs, "network-blockcidrs", "", "Comma-separated values for seed.Spec.Networks.BlockCIDRs.")

	seedCreationConfig = newCfg

	return seedCreationConfig
}

func (f *SeedCreationFramework) CreateSeed(ctx context.Context) error {
	if f.GardenClient == nil {
		return errors.New("no gardener client is defined")
	}

	var (
		//seedClient kubernetes.Interface
		seed = &gardencorev1beta1.Seed{}
		err  error
	)
	seed.Name = f.Config.SeedName
	seed.Spec.DNS = gardencorev1beta1.SeedDNS{
		IngressDomain: f.Config.IngressDomain,
	}
	seed.Spec.SecretRef = &corev1.SecretReference{
		Name:      f.Config.NewSecretName,
		Namespace: f.Config.NewSecretNamespace,
	}

	seed.Labels = make(map[string]string)
	seed.Labels["gardener.cloud/role"] = "seed"

	seed.Spec.Backup = &gardencorev1beta1.SeedBackup{}
	seed.Spec.Backup.SecretRef = corev1.SecretReference{}
	seed.Spec.Backup.SecretRef.Name = f.Config.BackupSecretName
	seed.Spec.Backup.SecretRef.Namespace = f.Config.BackupSecretNamespace
	seed.Spec.Backup.Provider = f.Config.BackupSecretProvider

	seed.Spec.Provider.Region = f.Config.Provider
	seed.Spec.Provider.Type = f.Config.ProviderType

	refShoot := gardencorev1beta1.Shoot{}

	if err = f.GardenClient.DirectClient().Get(ctx, kutil.Key(f.Config.ShootedSeedNamespace, f.Config.ShootedSeedName), &refShoot); err != nil {
		return err
	}

	seed.Spec.Networks = gardencorev1beta1.SeedNetworks{}
	seed.Spec.Networks.BlockCIDRs = strings.Split(f.Config.BlockCIDRs, ",")
	seed.Spec.Networks.Nodes = refShoot.Spec.Networking.Nodes
	seed.Spec.Networks.Pods = *refShoot.Spec.Networking.Pods
	seed.Spec.Networks.Services = *refShoot.Spec.Networking.Services
	podsDef := f.Config.NetworkDefPods
	seed.Spec.Networks.ShootDefaults = &gardencorev1beta1.ShootNetworks{}
	seed.Spec.Networks.ShootDefaults.Pods = &podsDef
	servicesDef := f.Config.NetworkDefServices
	seed.Spec.Networks.ShootDefaults.Services = &servicesDef
	seed.Spec.Volume = &gardencorev1beta1.SeedVolume{
		MinimumSize: resource.NewScaledQuantity(5, resource.Giga),
	}
	seed.Spec.Settings = &gardencorev1beta1.SeedSettings{
		Scheduling: &gardencorev1beta1.SeedSettingScheduling{
			Visible: false,
		},
	}

	f.Seed = seed
	err = f.GenerateSeedSecret(ctx)

	fmt.Println("=====Seed configuration:")
	PrettyPrintObject(f.Secret)
	fmt.Println("---------")
	PrettyPrintObject(seed)

	//Apply secret to the cluster
	_, err = f.createSeedSecret(ctx, f.Secret)
	//Apply the seed
	err = f.GardenerFramework.CreateSeed(ctx, f.Seed)

	return err
}

func (f *SeedCreationFramework) createSeedSecret(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	_, err := f.GetSecret(ctx, secret.Name, secret.Namespace)
	if err == nil {
		return secret, apierrors.NewAlreadyExists(gardencorev1beta1.Resource("secret"), secret.Name)
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	if err := f.GardenClient.DirectClient().Create(ctx, secret); err != nil {
		return nil, err
	}
	f.Logger.Infof("Secret resource %s was created!", secret.Name)
	return secret, nil
}

// createShootResource creates a shoot from a shoot Object
func (f *GardenerFramework) createSeedResource(ctx context.Context, seed *gardencorev1beta1.Seed) (*gardencorev1beta1.Seed, error) {
	err := f.checkIfSeedExists(ctx, seed)
	if err == nil {
		return seed, apierrors.NewAlreadyExists(gardencorev1beta1.Resource("seed"), seed.Name)
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	if err := f.GardenClient.DirectClient().Create(ctx, seed); err != nil {
		f.Logger.Errorf("Failed to create SEED: ", err)
		return nil, err
	}
	f.Logger.Infof("Seed resource %s was created!", seed.Name)
	return seed, nil
}

func (f *GardenerFramework) checkIfSeedExists(ctx context.Context, seed *gardencorev1beta1.Seed) error {
	return f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Name: seed.Name}, seed)
}

// CreateShoot Creates a shoot from a seed Object and waits until it is successfully reconciled
func (f *GardenerFramework) CreateSeed(ctx context.Context, seed *gardencorev1beta1.Seed) error {
	err := retry.UntilTimeout(ctx, 20*time.Second, 5*time.Minute, func(ctx context.Context) (done bool, err error) {
		_, err = f.createSeedResource(ctx, seed)
		if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsAlreadyExists(err) {
			return retry.SevereError(err)
		}
		if err != nil {
			f.Logger.Debugf("unable to create seed %s: %s", seed.Name, err.Error())
			return retry.MinorError(err)
		}
		return retry.Ok()
	})
	if err != nil {
		return err
	}

	// Then we wait for the shoot to be created
	err = f.WaitForSeedToBeCreated(ctx, seed)
	if err != nil {
		return err
	}

	f.Logger.Infof("Seed %s was created!", seed.Name)
	return nil
}

func (f *SeedCreationFramework) GenerateSeedSecret(ctx context.Context) (e error) {
	var (
		secret        = corev1.Secret{}
		refSecret     = corev1.Secret{}
		kubecfgSecret = corev1.Secret{}
		kubeconfig    []byte
	)

	if err := f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Namespace: f.Config.ShootedSeedNamespace, Name: f.Config.ShootedSeedName + ".kubeconfig"}, &kubecfgSecret); err != nil {
		fmt.Println("Unable to get kubeconfig from secret", err)
		kcfg, err := getBytesFromFile(f.Config.ShootedSeedKubeconfig)
		if err != nil {
			e = errors.Wrapf(err, "could not get shoot kubeconfig")
		}
		kubeconfig = kcfg
	} else {
		fmt.Println("Kubecfg succesfully fetched from secret")
		kcfg, ex := kubecfgSecret.Data[kubeconfigString]
		if !ex {
			return err
		}
		kubeconfig = kcfg
	}

	if err := f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Namespace: f.Config.SecretRefNamespace, Name: f.Config.SecretRefName}, &refSecret); err != nil {
		e = errors.Wrapf(err, "could not get shoot kubeconfig secret")
	}

	secret.Name = f.Config.NewSecretName
	secret.Namespace = f.Config.NewSecretNamespace
	secret.Data = make(map[string][]byte)

	for k, v := range refSecret.Data {
		secret.Data[k] = v
	}
	secret.Data[kubeconfigString] = kubeconfig

	f.Secret = &secret

	return e
}

func getBytesFromFile(filePath string) (b64 []byte, err error) {
	return ioutil.ReadFile(filePath)
}
