package framework

import (
	"context"
	"flag"
	"os"
	"reflect"
	"sort"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/onsi/ginkgo"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var shootMigrationConfig *ShootMigrationConfig

const (
	SecretName              = "test-shoot-migration-secret"
	SecretNamespace         = "default"
	ServiceAccountName      = "test-service-account"
	ServiceAccountNamespace = "default"
)

// SeedCreationFramework represents the seed test framework that includes
// test functions that can be executed ona specific seed
type ShootMigrationFramework struct {
	*GardenerFramework
	TestDescription
	Config             *ShootMigrationConfig
	TargetSeedClient   kubernetes.Interface
	SourceSeedClient   kubernetes.Interface
	ShootClient        kubernetes.Interface
	TargetSeed         *gardencorev1beta1.Seed
	SourceSeed         *gardencorev1beta1.Seed
	ComparisonElements ShootComparisonElemets
	Shoot              gardencorev1beta1.Shoot
}

// SeedCreationConfig is the configuration for a seed framework that will be filled with user provided data
type ShootMigrationConfig struct {
	GardenerConfig *GardenerConfig
	TargetSeedName string
	SourceSeedName string
	ShootName      string
	ShootNamespace string
}

type ShootComparisonElemets struct {
	MachineNamesBeforeMigration []string
	NodeNamesBeforeMigration    []string
	MachineNamesAfterMigration  []string
	MachineNodesAfterMigration  []string
	NodeNamesAfterMigration     []string
	PodStatusBeforeMigration    map[string]corev1.PodPhase
	PodStatusAfterMigration     map[string]corev1.PodPhase
}

// NewSeedCreationFramework creates a new simple Seed framework
func NewShootMigrationFramework(cfg *ShootMigrationConfig) *ShootMigrationFramework {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}

	f := &ShootMigrationFramework{
		GardenerFramework: NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   NewTestDescription("Shoot Migration"),
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
func NewShootMigrationFrameworkFromConfig(cfg *ShootMigrationConfig) (*ShootMigrationFramework, error) {
	var gardenerConfig *GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}
	f := &ShootMigrationFramework{
		GardenerFramework: NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   NewTestDescription("SEED"),
		Config:            cfg,
	}
	return f, nil
}

// BeforeEach should be called in ginkgo's BeforeEach.
// It sets up the seed framework.
func (f *ShootMigrationFramework) BeforeEach(ctx context.Context) {
	f.Config = mergeConfig(f.Config, shootMigrationConfig)
	validateConfig(f.Config)
	if err := f.initShoot(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	if err := f.initSeedsAndClients(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	f.ComparisonElements = ShootComparisonElemets{}
	if err := f.populateBeforeMigrationComparisonElements(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	if err := f.createTestSecret(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	if err := f.createTestServiceAccount(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	podStatusMap, err := f.getPodsStatus(ctx)
	if err != nil {
		ginkgo.Fail(err.Error())
	}
	f.ComparisonElements.PodStatusBeforeMigration = podStatusMap
}

// AfterEach should be called in ginkgo's AfterEach.
// Cleans up resources and dumps the shoot state if the test failed
func (f *ShootMigrationFramework) AfterEach(ctx context.Context) {
	if ginkgo.CurrentGinkgoTestDescription().Failed {
		f.DumpState(ctx)
	}
	if err := f.populateAfterMigrationComparisonElements(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}

	if err := f.deleteTestSecret(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	if err := f.deleteTestServiceAccount(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	podStatusMap, err := f.getPodsStatus(ctx)
	if err != nil {
		ginkgo.Fail(err.Error())
	}
	f.ComparisonElements.PodStatusAfterMigration = podStatusMap
	if err := f.compareElementsAfterMigration(); err != nil {
		ginkgo.Fail(err.Error())
	}
}

func validateConfig(cfg *ShootMigrationConfig) {
	if cfg == nil {
		ginkgo.Fail("no shoot framework configuration provided")
	}
	if !StringSet(cfg.TargetSeedName) {
		ginkgo.Fail("You should specify a name for the new Seed")
	}
	if !StringSet(cfg.ShootName) {
		ginkgo.Fail("You should specify a name for the new Shoot")
	}
	if !StringSet(cfg.ShootNamespace) {
		ginkgo.Fail("You should specify a namespace for the new Shoot")
	}
}

func mergeConfig(base, overwrite *ShootMigrationConfig) *ShootMigrationConfig {
	if base == nil {
		return overwrite
	}
	if overwrite == nil {
		return base
	}
	if overwrite.GardenerConfig != nil {
		base.GardenerConfig = overwrite.GardenerConfig
	}
	if StringSet(overwrite.TargetSeedName) {
		base.TargetSeedName = overwrite.TargetSeedName
	}
	if StringSet(overwrite.ShootName) {
		base.ShootName = overwrite.ShootName
	}
	if StringSet(overwrite.ShootNamespace) {
		base.ShootNamespace = overwrite.ShootNamespace
	}
	return base
}

// RegisterSeedCreationFrameworkFlags adds all flags that are needed to configure a shoot framework to the provided flagset.
func RegisterShootMigrationFrameworkFlags() *SeedCreationConfig {
	_ = RegisterGardenerFrameworkFlags()

	newCfg := &ShootMigrationConfig{}

	flag.StringVar(&newCfg.TargetSeedName, "target-seed-name", "", "name of the seed")
	flag.StringVar(&newCfg.ShootName, "shoot-name", "", "name of the shoot")
	flag.StringVar(&newCfg.ShootNamespace, "shoot-namespace", "", "namespace of the shoot")

	shootMigrationConfig = newCfg

	return seedCreationConfig
}

func (f *ShootMigrationFramework) MigrateShoot(ctx context.Context) error {
	// Dump gardener state if delete shoot is in exit handler
	if os.Getenv("TM_PHASE") == "Exit" {
		if seedFramework, err := f.NewShootFramework(&f.Shoot); err == nil {
			seedFramework.DumpState(ctx)
		} else {
			f.DumpState(ctx)
		}
	}

	//DO NOT DELETE
	if err := f.GardenerFramework.MigrateShoot(ctx, &f.Shoot, f.TargetSeed); err != nil {
		return err
	}

	return nil
}

func (f *ShootMigrationFramework) getNodeNames(ctx context.Context, seedClient kubernetes.Interface) (nodeNames []string, err error) {
	nodeList := v1.NodeList{}
	if err := seedClient.Client().List(ctx, &nodeList, client.InNamespace("shoot--"+f.Config.ShootNamespace+"--"+f.Config.ShootName)); err != nil {
		return nil, err
	}

	nodeNames = make([]string, len(nodeList.Items))
	for i, node := range nodeList.Items {
		f.Logger.Infof("%v. %v", i, node.Name)
		nodeNames[i] = node.Name
	}
	sort.Strings(nodeNames)
	return
}

func (f *ShootMigrationFramework) getMachineDetails(ctx context.Context, seedClient kubernetes.Interface) (machineNames, machineNodes []string, err error) {
	machineList := unstructured.UnstructuredList{}
	machineList.SetKind("Machine")
	machineList.SetAPIVersion("machine.sapcloud.io/v1alpha1")
	if err := seedClient.Client().List(ctx, &machineList, client.InNamespace("shoot--"+f.Config.ShootNamespace+"--"+f.Config.ShootName)); err != nil {
		return nil, nil, err
	}

	machineNames = make([]string, len(machineList.Items))
	machineNodes = make([]string, len(machineList.Items))
	for i, machine := range machineList.Items {
		f.Logger.Infof("%v. Neme: %v, Node: %v", i, machine.GetName(), machine.GetLabels()["node"])
		machineNames[i] = machine.GetName()
		machineNodes[i] = machine.GetLabels()["node"]
	}
	sort.Strings(machineNames)
	sort.Strings(machineNodes)
	return
}

func (f *ShootMigrationFramework) initShoot(ctx context.Context) (err error) {
	shoot := &gardencorev1beta1.Shoot{ObjectMeta: metav1.ObjectMeta{Name: f.Config.ShootName, Namespace: f.Config.ShootNamespace}}
	err = f.GardenerFramework.GetShoot(ctx, shoot)

	kubecfgSecret := corev1.Secret{}
	if err := f.GetSecret(ctx, shoot.Name+".kubeconfig", shoot.Namespace, &kubecfgSecret); err != nil {
		f.Logger.Errorf("Unable to get kubeconfig from secret", err)
		return err
	}
	f.Logger.Info("Shoot kubeconfig secret was fetched successfully")
	f.ShootClient, err = kubernetes.NewClientFromSecret(ctx, f.GardenClient.DirectClient(), kubecfgSecret.Namespace, kubecfgSecret.Name, kubernetes.WithClientOptions(client.Options{
		Scheme: kubernetes.ShootScheme,
	}))
	f.Shoot = *shoot
	return
}

func (f *ShootMigrationFramework) initSeedsAndClients(ctx context.Context) error {
	f.Config.SourceSeedName = *f.Shoot.Spec.SeedName

	if seed, seedClient, err := f.GetSeed(ctx, f.Config.TargetSeedName); err != nil {
		return err
	} else {
		f.TargetSeedClient = seedClient
		f.TargetSeed = seed
	}

	if seed, seedClient, err := f.GetSeed(ctx, f.Config.SourceSeedName); err != nil {
		return err
	} else {
		f.SourceSeedClient = seedClient
		f.SourceSeed = seed
	}
	return nil
}

func (f *ShootMigrationFramework) populateBeforeMigrationComparisonElements(ctx context.Context) (err error) {
	f.ComparisonElements.MachineNamesBeforeMigration, _, err = f.getMachineDetails(ctx, f.SourceSeedClient)
	f.ComparisonElements.NodeNamesBeforeMigration, err = f.getNodeNames(ctx, f.ShootClient)
	return
}

func (f *ShootMigrationFramework) populateAfterMigrationComparisonElements(ctx context.Context) (err error) {
	f.ComparisonElements.MachineNamesAfterMigration, f.ComparisonElements.MachineNodesAfterMigration, err = f.getMachineDetails(ctx, f.TargetSeedClient)
	f.ComparisonElements.NodeNamesAfterMigration, err = f.getNodeNames(ctx, f.ShootClient)
	return
}

func (f *ShootMigrationFramework) compareElementsAfterMigration() error {
	if !reflect.DeepEqual(f.ComparisonElements.MachineNamesBeforeMigration, f.ComparisonElements.MachineNamesAfterMigration) {
		return errors.Errorf("Initial Machines %v, does not match after-migrate Machines %v", f.ComparisonElements.MachineNamesBeforeMigration, f.ComparisonElements.MachineNamesAfterMigration)
	}
	if !reflect.DeepEqual(f.ComparisonElements.NodeNamesBeforeMigration, f.ComparisonElements.NodeNamesAfterMigration) {
		return errors.Errorf("Initial Nodes %v, does not match after-migrate Nodes %v", f.ComparisonElements.NodeNamesBeforeMigration, f.ComparisonElements.NodeNamesAfterMigration)
	}
	if !reflect.DeepEqual(f.ComparisonElements.MachineNodesAfterMigration, f.ComparisonElements.NodeNamesAfterMigration) {
		return errors.Errorf("Machine nodes (label) %v, does not match after-migrate Nodes %v", f.ComparisonElements.MachineNodesAfterMigration, f.ComparisonElements.NodeNamesAfterMigration)
	}
	if !reflect.DeepEqual(f.ComparisonElements.PodStatusBeforeMigration, f.ComparisonElements.PodStatusAfterMigration) {
		return errors.Errorf("Pod status before-migration %v, does not match after-migration Pods %v", f.ComparisonElements.MachineNodesAfterMigration, f.ComparisonElements.NodeNamesAfterMigration)
	}
	return nil
}

func (f *ShootMigrationFramework) checkPodsStatus(ctx context.Context) error {

	return nil
}

func (f *ShootMigrationFramework) createTestSecret(ctx context.Context) error {
	var secret = &corev1.Secret{}

	err := f.ShootClient.DirectClient().Get(ctx, client.ObjectKey{Name: SecretName, Namespace: SecretNamespace}, secret)

	if err == nil {
		return apierrors.NewAlreadyExists(gardencorev1beta1.Resource("secret"), SecretName)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	secret.Name = SecretName
	secret.Namespace = SecretNamespace

	if err := f.ShootClient.DirectClient().Create(ctx, secret); err != nil {
		return err
	}
	f.Logger.Infof("Secret resource %s was created!", SecretName)
	return nil
}

func (f *ShootMigrationFramework) deleteTestSecret(ctx context.Context) error {
	var secret = &corev1.Secret{}
	if err := f.ShootClient.DirectClient().Get(ctx, client.ObjectKey{Name: SecretName, Namespace: SecretNamespace}, secret); err != nil {
		return err
	}

	if err := f.ShootClient.DirectClient().Delete(ctx, secret); err != nil {
		return err
	}
	f.Logger.Infof("Secret resource %s was deleted!", SecretName)
	return nil
}

func (f *ShootMigrationFramework) createTestServiceAccount(ctx context.Context) error {
	var serviceAccount = &corev1.ServiceAccount{}

	err := f.ShootClient.DirectClient().Get(ctx, client.ObjectKey{Name: ServiceAccountName, Namespace: ServiceAccountNamespace}, serviceAccount)
	if err == nil {
		return apierrors.NewAlreadyExists(gardencorev1beta1.Resource("serviceaccount"), ServiceAccountName)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	serviceAccount.Name = ServiceAccountName
	serviceAccount.Namespace = ServiceAccountNamespace

	if err := f.ShootClient.DirectClient().Create(ctx, serviceAccount); err != nil {
		return err
	}
	f.Logger.Infof("ServiceAccount %s was created!", ServiceAccountName)
	return nil
}

func (f *ShootMigrationFramework) deleteTestServiceAccount(ctx context.Context) error {
	var serviceAccount = &corev1.ServiceAccount{}

	if err := f.ShootClient.DirectClient().Get(ctx, client.ObjectKey{Name: ServiceAccountName, Namespace: ServiceAccountNamespace}, serviceAccount); err != nil {
		return err
	}

	if err := f.ShootClient.DirectClient().Delete(ctx, serviceAccount); err != nil {
		return err
	}
	f.Logger.Infof("ServiceAccount %s was deleted!", ServiceAccountName)
	return nil
}
func (f *ShootMigrationFramework) getPodsStatus(ctx context.Context) (map[string]corev1.PodPhase, error) {
	podList := corev1.PodList{}
	if err := f.ShootClient.Client().List(ctx, &podList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		return nil, err
	}
	podStatusMap := make(map[string]corev1.PodPhase, len(podList.Items))
	for i, pod := range podList.Items {
		f.Logger.Infof("%d, %v/%v : %v", i, pod.Namespace, pod.Name, pod.Status.Phase)
		podStatusMap[pod.Namespace+"/"+pod.Name] = pod.Status.Phase
	}
	return podStatusMap, nil
}
