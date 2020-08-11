# Gardener Scheduler

The Gardener Scheduler is in essence a controller that watches newly created shoots and assigns a seed cluster to them.
Conceptually, the task of the Gardener Scheduler is very similar to the task of the Kubernetes Scheduler: finding a seed for a shoot instead of a node for a pod.

Either the scheduling strategy or the shoot cluster purpose hereby determines how the scheduler is operating.
The following sections explain the configuration and flow in greater detail.

## Why is the Gardener Scheduler needed?

### 1. Decoupling

Previously, an admission plugin in the Gardener API server conducted the scheduling decisions.
This implies changes to the API server whenever adjustments of the scheduling are needed.
Decoupling the API server and the scheduler comes with greater flexibility to develop these components independently from each other.

### 2. Extensibility

It should be possible to easily extend and tweak the scheduler in the future.
Possibly, similar to the Kubernetes scheduler, hooks could be provided which influence the scheduling decisions.
It should be also possible to completely replace the standard Gardener Scheduler with a custom implementation.

## Configuration

The Gardener Scheduler configuration has to be supplied on startup. It is a mandatory and also the only available flag.
[Here](../../example/20-componentconfig-gardener-scheduler.yaml) is an example scheduler configuration.

Most of the configuration options are the same as in the Gardener Controller Manager (leader election, client connection, ...).
However, the Gardener Scheduler on the other hand does not need a TLS configuration, because there are currently no webhooks configurable.

The scheduling strategy is defined in the _**candidateDeterminationStrategy**_ and can have the possible values `SameRegion` and `MinimalDistance`.
The `SameRegion` strategy is the default strategy.

1. *Same Region strategy*

   The Gardener Scheduler reads the `spec.provider.type` and `.spec.region` fields from the `Shoot` resource.
It tries to find a Seed that has the identical `.spec.provider.type` and `.spec.provider.region` fields set.
If it cannot find a suitable Seed, it adds an event to the shoot stating, that it is unschedulable.

2. *Minimal Distance strategy*

   The Gardener Scheduler tries to find a valid seed with minimal distance to the shoot's intended region.
The distance is calculated based on the Levenshtein distance of the region. Therefore the region name
is split into a base name and an orientation. Possible orientations are `north`, `south`, `east`, `west` and `central`.
The distance then is twice the Levenshtein distance of the region's base name plus a correction value based on the
orientation and the provider.

   If the orientations of shoot and seed candidate match, the correction value is 0, if they differ it is 2 and if
either the seed's or the shoot's region does not have an orientation it is 1.
If the provider differs the correction value is additionally incremented by 2.

   Because of this a matching region with a matching provider is always prefered.

In the last step, the scheduler picks the one seed having the least shoots currently deployed.

In order to put the scheduling decision into effect, the scheduler sends an update request for the `Shoot` resource to
the API server. After validation, the Gardener Aggregated API server updates the shoot to have the `spec.seedName` field set.
Subsequently, the Gardenlet picks up and starts to create the cluster on the specified seed.

### Special handling based on shoot cluster purpose

Every shoot cluster can have a purpose that describes what the cluster is used for, and also influences how the cluster is setup (see [this document](../usage/shoot_purposes.md) for more information).

In case the shoot has the `testing` purpose then the scheduler only reads the `.spec.provider.type` from the `Shoot` resource and tries to find a `Seed` that has the identical `.spec.provider.type`.
The region does not matter, i.e., `testing` shoots may also be scheduled on a seed in a complete different region if it is better for balancing the whole Gardener system.

## Filtering to determine the best candidate

The section above has explained which strategies are used to determine the potential seed candidates.
Once this list has been computed the scheduler tries to find the best one out of them to which, eventually, the shoot gets assigned to.
It filters out `Seed`s

* whose networks have intersections with the `Shoot`'s networks (due to the VPN connectivity between seeds and shoots their networks must be disjoint)
* that are tainted with the `seed.gardener.cloud/disable-dns` taint (only if the shoot specifies a DNS domain or does not use the `unmanaged` DNS provider)
* whose labels don't match the `.spec.seedSelector` field of the `CloudProfile` that is used in the `Shoot` (there might be multiple environments for the same provider type, e.g., you might have multiple OpenStack systems connected to Gardener)

After this filtering process the least utilized seed, i.e., the one with the least number of shoot control planes, will be the winner and written to the `.spec.seedName` field of the `Shoot`.

## `seedSelector` field in the `Shoot` specification

Similar to the `.spec.nodeSelector` field in `Pod`s, the `Shoot` specification has an optional `.spec.seedSelector` field.
It allows the user to provide a label selector that must match the labels of `Seed`s in order to be scheduled to one of them.
The labels on `Seed`s are usually controlled by Gardener administrators/operators - end users cannot add arbitrary labels themselves.
If provided, the Gardener Scheduler will only consider those seeds as "suitable" whose labels match those provided in the `.spec.seedSelector` of the `Shoot`.

By default only seeds with the same provider than the shoot are selected. By adding a `providerTypes` field to the `seedSelector`
a dedicated set of possible providers (`*` means all provider types) can be selected.

## Failure to determine a suitable seed

In case the scheduler fails to find a suitable seed, the operation is being retried with an exponential backoff - starting with the `retrySyncPeriod` (default of `15s`).

## Current Limitation / Future Plans

- Azure has unfortunately a geographically non-hierarchical naming pattern and does not start with the continent. This is the reason why we will exchange the implementation of the `MinimalRegion` strategy with a more suitable one in the future.
