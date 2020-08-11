package framework

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	"github.com/onsi/ginkgo"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var seedCreationConfig *SeedCreationConfig

const (
	kubeconfigString = "kubeconfig"
)

// SeedFramework represents the seed test framework that includes
// test functions that can be executed ona specific seed
type SeedFramework struct {
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
	SeedName              string //e.g. test-aws
	IngressDomain         string //e.g. ingress.seed-aws.i/d<User>.shoot.dev.k8s-hana.ondemand.com
	SecretRefName         string //e.g. seed-test-aws
	SecretRefNamespace    string //e.g. garden
	NewSecretName         string
	NewSecretNamespace    string
	ShootedSeedName       string
	ShootedSeedNamespace  string
	ShootedSeedKubeconfig string
	SeedScheme            *runtime.Scheme
}

// NewSeedFramework creates a new simple Shoot framework
func NewSeedCreationFramework(cfg *SeedCreationConfig) *SeedFramework {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}

	f := &SeedFramework{
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

// NewSeedFrameworkFromConfig creates a new seed framework from a seed configuration without registering ginkgo
// specific functions
func NewSeedFrameworkFromConfig(cfg *SeedCreationConfig) (*SeedFramework, error) {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}
	f := &SeedFramework{
		GardenerFramework: NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   NewTestDescription("SEED"),
		Config:            cfg,
	}
	if cfg != nil && gardenerConfig != nil {

	}
	return f, nil
}

// BeforeEach should be called in ginkgo's BeforeEach.
// It sets up the shoot framework.
func (f *SeedFramework) BeforeEach(ctx context.Context) {
	f.Config = mergeSeedConfig(f.Config, seedCreationConfig)
	validateSeedConfig(f.Config)
	//err := f.AddShoot(ctx, f.Config.SeedName, f.ProjectNamespace)
	//1. create secret
	//2. create seed
	//f.CreateSeed(ctx)
}

// AfterEach should be called in ginkgo's AfterEach.
// Cleans up resources and dumps the shoot state if the test failed
func (f *SeedFramework) AfterEach(ctx context.Context) {
	if ginkgo.CurrentGinkgoTestDescription().Failed {
		f.DumpState(ctx)
	}
	//check result
}

func validateSeedConfig(cfg *SeedCreationConfig) {
	if cfg == nil {
		ginkgo.Fail("no shoot framework configuration provided")
	}
	if !StringSet(cfg.SeedName) {
		ginkgo.Fail("You should specify a SeedName to test against")
	}
	if !StringSet(cfg.IngressDomain) {
		ginkgo.Fail("You should specify a SeedName to test against")
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
	if StringSet(overwrite.ShootedSeedName) {
		base.ShootedSeedName = overwrite.ShootedSeedName
	}
	if StringSet(overwrite.ShootedSeedNamespace) {
		base.ShootedSeedNamespace = overwrite.ShootedSeedNamespace
	}
	if StringSet(overwrite.ShootedSeedKubeconfig) {
		base.ShootedSeedKubeconfig = overwrite.ShootedSeedKubeconfig
	}
	return base
}

// RegisterSeedFrameworkFlags adds all flags that are needed to configure a shoot framework to the provided flagset.
func RegisterSeedFrameworkFlags() *SeedCreationConfig {
	_ = RegisterGardenerFrameworkFlags()

	newCfg := &SeedCreationConfig{}

	flag.StringVar(&newCfg.SeedName, "seed-name", "", "name of the seed")
	flag.StringVar(&newCfg.IngressDomain, "ingress-domain", "", "ingress domain")
	flag.StringVar(&newCfg.SecretRefName, "secret-ref-name", "", "name of the secret reference")
	flag.StringVar(&newCfg.SecretRefNamespace, "secret-ref-namespace", "", "namespace of the secret reference")
	flag.StringVar(&newCfg.NewSecretName, "new-secret-name", "", "name of the new secret reference")
	flag.StringVar(&newCfg.NewSecretNamespace, "new-secret-namespace", "", "namespace of the new secret reference")
	flag.StringVar(&newCfg.ShootedSeedName, "shooted-seed-name", "", "name of the seed")
	flag.StringVar(&newCfg.ShootedSeedNamespace, "shooted-seed-namespace", "", "name of the seed")
	flag.StringVar(&newCfg.ShootedSeedKubeconfig, "shooted-seed-kubecfg", "", "kubeconfig of the shooted seed")

	seedCreationConfig = newCfg

	return seedCreationConfig
}

func (f *SeedFramework) CreateSeed(ctx context.Context) error {
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

	//TODO backup props
	seed.Spec.Backup = &gardencorev1beta1.SeedBackup{}
	seed.Spec.Backup.SecretRef = corev1.SecretReference{}
	seed.Spec.Backup.SecretRef.Name = "backup-aws"
	seed.Spec.Backup.SecretRef.Namespace = "garden"
	seed.Spec.Backup.Provider = "aws"

	//TODO region props
	seed.Spec.Provider.Region = "eu-west-1"
	seed.Spec.Provider.Type = "aws"

	//TODO network
	nodesDef := "10.222.0.0/16"
	seed.Spec.Networks = gardencorev1beta1.SeedNetworks{}
	seed.Spec.Networks.BlockCIDRs = []string{"169.254.169.254/32"}
	seed.Spec.Networks.Nodes = &nodesDef
	seed.Spec.Networks.Pods = "10.223.128.0/17"
	seed.Spec.Networks.Services = "10.223.0.0/17"
	podsDef := "100.96.0.0/11"
	seed.Spec.Networks.ShootDefaults = &gardencorev1beta1.ShootNetworks{}
	seed.Spec.Networks.ShootDefaults.Pods = &podsDef
	servicesDef := "100.64.0.0/13"
	seed.Spec.Networks.ShootDefaults.Services = &servicesDef
	seed.Spec.Volume = &gardencorev1beta1.SeedVolume{
		MinimumSize: resource.NewScaledQuantity(20, resource.Giga),
	}

	f.Seed = seed
	err = f.CreateSeedSecret(ctx)

	fmt.Println("=====Seed configuration:")
	PrettyPrintObject(f.Secret)
	fmt.Println("---------")
	PrettyPrintObject(seed)

	//Apply secret to the cluster
	_, err = f.createSeedSecret(ctx, f.Secret)
	//Apply the seed
	f.GardenerFramework.CreateSeed(ctx, f.Seed)

	return err
}

func (f *SeedFramework) createSeedSecret(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
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

// GetSecret returns the seed and its k8s client
func (f *GardenerFramework) GetSecret(ctx context.Context, seedName, seedNamespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Name: seedName, Namespace: seedNamespace}, secret)
	if err != nil {
		return nil, err
	}
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

// CreateShoot Creates a shoot from a shoot Object and waits until it is successfully reconciled
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

	f.Logger.Infof("Shoot %s was created!", seed.Name)
	return nil
}

// WaitForShootToBeCreated waits for the shoot to be created
func (f *GardenerFramework) WaitForSeedToBeCreated(ctx context.Context, seed *gardencorev1beta1.Seed) error {
	return retry.UntilTimeout(ctx, 30*time.Second, 60*time.Minute, func(ctx context.Context) (done bool, err error) {
		newSeed := &gardencorev1beta1.Seed{}
		err = f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Name: seed.Name}, newSeed)
		if err != nil {
			f.Logger.Infof("Error while waiting for seed to be created: %s", err.Error())
			return retry.MinorError(err)
		}
		*seed = *newSeed
		completed, msg := SeedCreationCompleted(seed)
		if completed {
			return retry.Ok()
		}
		f.Logger.Infof("Shoot %s not yet created successfully (%s)", seed.Name, msg)

		return retry.MinorError(fmt.Errorf("shoot %q was not successfully reconciled", seed.Name))
	})
}

// ShootCreationCompleted checks if a shoot is successfully reconciled. In case it is not, it also returns a descriptive message stating the reason.
func SeedCreationCompleted(newSeed *gardencorev1beta1.Seed) (bool, string) {
	if newSeed.Generation != newSeed.Status.ObservedGeneration {
		return false, "shoot generation did not equal observed generation"
	}
	if len(newSeed.Status.Conditions) == 0 {
		return false, "no conditions and last operation present yet"
	}

	for _, condition := range newSeed.Status.Conditions {
		if condition.Status != gardencorev1beta1.ConditionTrue && condition.Reason != "BootstrappingSucceeded" {
			return false, fmt.Sprintf("condition type %s is not true yet, had message %s with reason %s", condition.Type, condition.Message, condition.Reason)
		}
	}
	return true, ""
}

func (f *SeedFramework) CreateSeedSecret(ctx context.Context) (e error) {
	var (
		secret     = corev1.Secret{}
		refSecret  = corev1.Secret{}
		kubeconfig []byte
	)

	if err := f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Namespace: f.Config.SecretRefNamespace, Name: f.Config.SecretRefName}, &refSecret); err != nil {
		e = errors.Wrapf(err, "could not get shoot kubeconfig secret")
	}

	//kubeconfig, err := getKubeconfigAsBase64(f.Config.ShootedSeedKubeconfig)
	kubeconfig, err := getBytesFromFile(f.Config.ShootedSeedKubeconfig)
	if err != nil {
		e = errors.Wrapf(err, "could not get shoot kubeconfig from file path")
	}

	//fmt.Println(string(kubeconfig))

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

func getKubeconfigAsBase64(filePath string) (b64 []byte, err error) {
	content, err := getBytesFromFile(filePath)

	// Convert []byte to string and print to screen
	b64 = []byte(base64.StdEncoding.EncodeToString(content))
	return b64, err
}

func getBytesFromFile(filePath string) (b64 []byte, err error) {
	return ioutil.ReadFile(filePath)
}
