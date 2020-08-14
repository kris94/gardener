// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
	Overview
		- Tests the deletion of a shoot
 **/

package seed_deletion

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gardener/gardener/test/framework"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var seedName *string

func init() {
	seedName = flag.String("seed-name", "", "name of the seed")
	framework.RegisterGardenerFrameworkFlags()
}

func validateFlags() {
	if !framework.StringSet(*seedName) {
		Fail("flag '--seed-name' needs to be specified")
	}
}

var _ = Describe("Seed deletion testing", func() {

	f := framework.NewGardenerFramework(nil)

	framework.CIt("Testing if Seed can be deleted", func(ctx context.Context) {
		validateFlags()
		seed := &gardencorev1beta1.Seed{ObjectMeta: metav1.ObjectMeta{Name: *seedName}}
		if err := f.GardenClient.DirectClient().Get(ctx, client.ObjectKey{Name: *seedName}, seed); err != nil {
			if apierrors.IsNotFound(err) {
				Skip("seed is already deleted")
			}
			Expect(err).ToNot(HaveOccurred())
		}

		// Dump gardener state if delete shoot is in exit handler
		if os.Getenv("TM_PHASE") == "Exit" {
			if seedFramework, err := f.NewSeedFramework(seed); err == nil {
				seedFramework.DumpState(ctx)
			} else {
				f.DumpState(ctx)
			}
		}

		if err := f.DeleteSeedAndWaitForDeletion(ctx, seed); err != nil && !apierrors.IsNotFound(err) {
			if shootFramework, err := f.NewSeedFramework(seed); err == nil {
				shootFramework.DumpState(ctx)
			}
			f.Logger.Fatalf("Cannot delete seed %s: %s", *seedName, err.Error())
		}

		if err := f.DeleteSecret(ctx, seed.Spec.SecretRef.Name, seed.Spec.SecretRef.Namespace); err != nil {
			err = errors.Wrapf(err, "Secret %v/%v can`t be deleted", seed.Spec.SecretRef.Namespace, seed.Spec.SecretRef.Name)
			Fail(err.Error())
		}
		fmt.Printf("Secret %v/%v deleted successfully\n", seed.Spec.SecretRef.Namespace, seed.Spec.SecretRef.Name)
	}, 1*time.Hour)
})
