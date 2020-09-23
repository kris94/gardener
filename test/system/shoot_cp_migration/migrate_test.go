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

package cp_migration_test

import (
	"context"
	"time"

	"github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/framework/cp_migration"
	. "github.com/onsi/ginkgo"
)

var seedName *string

const (
	CreateAndReconcileTimeout = 2 * time.Hour
)

func init() {
	cp_migration.RegisterShootMigrationTestFlags()
}

var _ = Describe("Shoot migration testing", func() {

	f := cp_migration.NewShootMigrationTest(&cp_migration.ShootMigrationConfig{
		GardenerConfig: &framework.GardenerConfig{
			CommonConfig: &framework.CommonConfig{
				ResourceDir: "../../framework/resources",
			},
		},
	})

	f.CIt("Create and Reconcile Seed", func(ctx context.Context) {
		err := f.MigrateShoot(ctx)
		framework.ExpectNoError(err)
	}, CreateAndReconcileTimeout)
})
