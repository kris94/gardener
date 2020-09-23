package cp_migration

import (
	"context"
	"flag"
	"os"
	"reflect"
	"sort"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	framework "github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/framework/applications"
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

// ShootMigrationTest represents the seed test framework that includes
// test functions that can be executed ona specific seed
type ShootMigrationTest struct {
	*framework.GardenerFramework
	framework.TestDescription
	Config                            *ShootMigrationConfig
	TargetSeedClient                  kubernetes.Interface
	SourceSeedClient                  kubernetes.Interface
	ShootClient                       kubernetes.Interface
	TargetSeed                        *gardencorev1beta1.Seed
	SourceSeed                        *gardencorev1beta1.Seed
	ComparisonElementsBeforeMigration ShootComparisonElemets
	ComparisonElementsAfterMigration  ShootComparisonElemets
	Shoot                             gardencorev1beta1.Shoot
	SeedShootNamespace                string
	GuestBookApp                      *applications.GuestBookTest
}

// ShootMigrationConfig is the configuration for a seed framework that will be filled with user provided data
type ShootMigrationConfig struct {
	GardenerConfig *framework.GardenerConfig
	TargetSeedName string
	SourceSeedName string
	ShootName      string
	ShootNamespace string
}

type ShootComparisonElemets struct {
	MachineNames []string
	MachineNodes []string
	NodeNames    []string
	PodStatus    map[string]corev1.PodPhase
}

// NewShootMigrationTest creates a new simple shoot migration test
func NewShootMigrationTest(cfg *ShootMigrationConfig) *ShootMigrationTest {
	var gardenerConfig *framework.GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}

	f := &ShootMigrationTest{
		GardenerFramework: framework.NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   framework.NewTestDescription("Shoot Migration"),
		Config:            cfg,
	}

	framework.CBeforeEach(func(ctx context.Context) {
		f.CommonFramework.BeforeEach()
		f.GardenerFramework.BeforeEach()
		f.BeforeEach(ctx)
	}, 8*time.Minute)
	framework.CAfterEach(f.AfterEach, 10*time.Minute)
	return f
}

// NewShootMigrationTestFromConfig creates a new shoot cp migration test from a configuration without registering ginkgo
// specific functions
func NewShootMigrationTestFromConfig(cfg *ShootMigrationConfig) (*ShootMigrationTest, error) {
	var gardenerConfig *framework.GardenerConfig
	if cfg != nil {
		gardenerConfig = cfg.GardenerConfig
	}
	f := &ShootMigrationTest{
		GardenerFramework: framework.NewGardenerFrameworkFromConfig(gardenerConfig),
		TestDescription:   framework.NewTestDescription("SEED"),
		Config:            cfg,
	}
	return f, nil
}

// BeforeEach should be called in ginkgo's BeforeEach.
// It sets up the seed framework.
func (f *ShootMigrationTest) BeforeEach(ctx context.Context) {
	f.Config = mergeConfig(f.Config, shootMigrationConfig)
	validateConfig(f.Config)
	if err := f.initShoot(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	f.SeedShootNamespace = framework.ComputeTechnicalID(f.Config.GardenerConfig.ProjectNamespace, &f.Shoot)

	if err := f.initSeedsAndClients(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	f.ComparisonElementsBeforeMigration = ShootComparisonElemets{}
	if err := f.populateBeforeMigrationComparisonElements(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	if err := f.createTestSecret(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	if err := f.createTestServiceAccount(ctx); err != nil {
		ginkgo.Fail(err.Error())
	}
	guesbookApp, err := f.deployGuestBookAp(ctx)
	if err != nil {
		ginkgo.Fail(err.Error())
	}
	guesbookApp.Test(ctx)
	f.GuestBookApp = guesbookApp
	podStatusMap, err := f.getPodsStatus(ctx)
	if err != nil {
		ginkgo.Fail(err.Error())
	}
	f.ComparisonElementsBeforeMigration.PodStatus = podStatusMap
}

// AfterEach should be called in ginkgo's AfterEach.
// Cleans up resources and dumps the shoot state if the test failed
func (f *ShootMigrationTest) AfterEach(ctx context.Context) {
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
	f.ComparisonElementsAfterMigration.PodStatus = podStatusMap
	if err := f.compareElementsAfterMigration(); err != nil {
		ginkgo.Fail(err.Error())
	}
	f.GuestBookApp.Test(ctx)
	f.GuestBookApp.Cleanup(ctx)
}

func validateConfig(cfg *ShootMigrationConfig) {
	if cfg == nil {
		ginkgo.Fail("no shoot framework configuration provided")
	}
	if !framework.StringSet(cfg.TargetSeedName) {
		ginkgo.Fail("You should specify a name for the new Seed")
	}
	if !framework.StringSet(cfg.ShootName) {
		ginkgo.Fail("You should specify a name for the new Shoot")
	}
	if !framework.StringSet(cfg.ShootNamespace) {
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
	if framework.StringSet(overwrite.TargetSeedName) {
		base.TargetSeedName = overwrite.TargetSeedName
	}
	if framework.StringSet(overwrite.ShootName) {
		base.ShootName = overwrite.ShootName
	}
	if framework.StringSet(overwrite.ShootNamespace) {
		base.ShootNamespace = overwrite.ShootNamespace
	}
	return base
}

// RegisterSeedCreationFrameworkFlags adds all flags that are needed to configure a shoot framework to the provided flagset.
func RegisterShootMigrationTestFlags() *ShootMigrationConfig {
	_ = framework.RegisterGardenerFrameworkFlags()

	newCfg := &ShootMigrationConfig{}

	flag.StringVar(&newCfg.TargetSeedName, "target-seed-name", "", "name of the seed")
	flag.StringVar(&newCfg.ShootName, "shoot-name", "", "name of the shoot")
	flag.StringVar(&newCfg.ShootNamespace, "shoot-namespace", "", "namespace of the shoot")

	shootMigrationConfig = newCfg

	return shootMigrationConfig
}

func (f *ShootMigrationTest) MigrateShoot(ctx context.Context) error {
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

func (f *ShootMigrationTest) getNodeNames(ctx context.Context, seedClient kubernetes.Interface) (nodeNames []string, err error) {
	nodeList := v1.NodeList{}
	f.Logger.Infof("Getting node names in namespace: %s", f.SeedShootNamespace)
	if err := seedClient.Client().List(ctx, &nodeList, client.InNamespace(f.SeedShootNamespace)); err != nil {
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

func (f *ShootMigrationTest) getMachineDetails(ctx context.Context, seedClient kubernetes.Interface) (machineNames, machineNodes []string, err error) {
	//machineList := machinev1alpha1.MachineList{} // time="2020-09-11T17:33:32+03:00" level=error msg="Error while getting machine details, no kind is registered for the type v1alpha1.MachineList in scheme \"github.com/gardener/gardener/pkg/client/kubernetes/types.go:51\""
	machineList := unstructured.UnstructuredList{}
	machineList.SetAPIVersion("machine.sapcloud.io/v1alpha1")
	machineList.SetKind("Machine")
	f.Logger.Infof("Getting machine details in namespace: %s", f.SeedShootNamespace)
	if err := seedClient.Client().List(ctx, &machineList, client.InNamespace(f.SeedShootNamespace)); err != nil {
		f.Logger.Errorf("Error while getting machine details, %s", err.Error())
		return nil, nil, err
	}

	f.Logger.Infof("Found: %d items", len(machineList.Items))

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

func (f *ShootMigrationTest) initShoot(ctx context.Context) (err error) {
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

func (f *ShootMigrationTest) initSeedsAndClients(ctx context.Context) error {
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

func (f *ShootMigrationTest) populateBeforeMigrationComparisonElements(ctx context.Context) (err error) {
	f.ComparisonElementsBeforeMigration.MachineNames, _, err = f.getMachineDetails(ctx, f.SourceSeedClient)
	f.ComparisonElementsBeforeMigration.NodeNames, err = f.getNodeNames(ctx, f.ShootClient)
	return
}

func (f *ShootMigrationTest) populateAfterMigrationComparisonElements(ctx context.Context) (err error) {
	f.ComparisonElementsAfterMigration.MachineNames, f.ComparisonElementsAfterMigration.MachineNodes, err = f.getMachineDetails(ctx, f.TargetSeedClient)
	f.ComparisonElementsAfterMigration.NodeNames, err = f.getNodeNames(ctx, f.ShootClient)
	return
}

func (f *ShootMigrationTest) compareElementsAfterMigration() error {
	if !reflect.DeepEqual(f.ComparisonElementsBeforeMigration.MachineNames, f.ComparisonElementsAfterMigration.MachineNames) {
		return errors.Errorf("Initial Machines %v, does not match after-migrate Machines %v", f.ComparisonElementsBeforeMigration.MachineNames, f.ComparisonElementsAfterMigration.MachineNames)
	}
	if !reflect.DeepEqual(f.ComparisonElementsBeforeMigration.NodeNames, f.ComparisonElementsAfterMigration.NodeNames) {
		return errors.Errorf("Initial Nodes %v, does not match after-migrate Nodes %v", f.ComparisonElementsBeforeMigration.NodeNames, f.ComparisonElementsAfterMigration.NodeNames)
	}
	if !reflect.DeepEqual(f.ComparisonElementsAfterMigration.MachineNodes, f.ComparisonElementsAfterMigration.NodeNames) {
		return errors.Errorf("Machine nodes (label) %v, does not match after-migrate Nodes %v", f.ComparisonElementsAfterMigration.MachineNodes, f.ComparisonElementsAfterMigration.NodeNames)
	}
	if !reflect.DeepEqual(f.ComparisonElementsBeforeMigration.PodStatus, f.ComparisonElementsAfterMigration.PodStatus) {
		return errors.Errorf("Pod status before-migration %v, does not match after-migration Pods %v", f.ComparisonElementsBeforeMigration.PodStatus, f.ComparisonElementsAfterMigration.PodStatus)
	}
	return nil
}

func (f *ShootMigrationTest) createTestSecret(ctx context.Context) error {
	var secret = &corev1.Secret{}

	err := f.ShootClient.DirectClient().Get(ctx, client.ObjectKey{Name: SecretName, Namespace: SecretNamespace}, secret)

	if err == nil {
		f.Logger.Warnf("Secret %s/%s already exists. It will be deleted and recreated...", SecretNamespace, SecretName)
		if err = f.deleteTestSecret(ctx); err != nil {
			return err
		}
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

func (f *ShootMigrationTest) deleteTestSecret(ctx context.Context) error {
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

func (f *ShootMigrationTest) createTestServiceAccount(ctx context.Context) error {
	var serviceAccount = &corev1.ServiceAccount{}

	err := f.ShootClient.DirectClient().Get(ctx, client.ObjectKey{Name: ServiceAccountName, Namespace: ServiceAccountNamespace}, serviceAccount)
	if err == nil {
		f.Logger.Warnf("Service Account %s/%s already exists. It will be deleted and recreated...", SecretNamespace, SecretName)
		if err = f.deleteTestServiceAccount(ctx); err != nil {
			return err
		}
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

func (f *ShootMigrationTest) deleteTestServiceAccount(ctx context.Context) error {
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
func (f *ShootMigrationTest) getPodsStatus(ctx context.Context) (map[string]corev1.PodPhase, error) {
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

func (f *ShootMigrationTest) deployGuestBookAp(ctx context.Context) (*applications.GuestBookTest, error) {
	sFramework := framework.ShootFramework{
		GardenerFramework: f.GardenerFramework,
		TestDescription:   f.TestDescription,
		Shoot:             &f.Shoot,
		Seed:              f.SourceSeed,
		ShootClient:       f.ShootClient,
		SeedClient:        f.SourceSeedClient,
	}
	if f.Shoot.Spec.Addons.NginxIngress.Enabled == false {
		if err := f.UpdateShoot(ctx, &f.Shoot, func(shoot *gardencorev1beta1.Shoot) error {
			if err := f.GetShoot(ctx, shoot); err != nil {
				return err
			}

			shoot.Spec.Addons.NginxIngress.Enabled = true
			return nil
		}); err != nil {
			return nil, err
		}
	}

	guestbookApp, err := applications.NewGuestBookTest(&sFramework)
	if err != nil {
		return nil, err
	}
	guestbookApp.DeployGuestBookApp(ctx)
	return guestbookApp, nil
}
