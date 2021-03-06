package broker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mongodb/mongodb-atlas-service-broker/pkg/atlas"
	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-cf/brokerapi/domain/apiresponses"
)

// idPrefix will be prepended to service and plan IDs to ensure their uniqueness.
const idPrefix = "aosb-cluster"

// providerNames contains all the available cloud providers on which clusters
// may be provisioned. The available instance sizes for each provider are
// fetched dynamically from the Atlas API.
var (
	providerNames = []string{"AWS", "GCP", "AZURE", "TENANT"}

	// Hardcode the instance sizes for shared instances
	sharedService = brokerapi.Service{
		ID:                   "aosb-cluster-service-tenant",
		Name:                 "mongodb-atlas-tenant",
		Description:          "Atlas cluster hosted on \"TENANT\"",
		Bindable:             true,
		InstancesRetrievable: false,
		BindingsRetrievable:  false,
		Metadata:             nil,
		PlanUpdatable:        true,
		Plans: []brokerapi.ServicePlan{
			brokerapi.ServicePlan{
				ID:          "aosb-cluster-plan-tenant-m2",
				Name:        "M2",
				Description: "Instance size \"M2\"",
			},
			brokerapi.ServicePlan{
				ID:          "aosb-cluster-plan-tenant-m5",
				Name:        "M5",
				Description: "Instance size \"M5\"",
			},
		},
	}
)

// applyWhitelist filters a given service, returning the service with only the
// whitelisted plans.
func applyWhitelist(svc brokerapi.Service, whitelistedPlans []string) brokerapi.Service {
	whitelistedSvc := svc
	plans := []brokerapi.ServicePlan{}
	for _, plan := range whitelistedSvc.Plans {
		for _, name := range whitelistedPlans {
			if plan.Name == name {
				plans = append(plans, plan)
				break
			}
		}
	}

	whitelistedSvc.Plans = plans
	return whitelistedSvc
}

// Services generates the service catalog which will be presented to consumers of the API.
func (b Broker) Services(ctx context.Context) ([]brokerapi.Service, error) {
	b.logger.Info("Retrieving service catalog")

	services := []brokerapi.Service{}
	client, err := atlasClientFromContext(ctx)
	if err != nil {
		return services, err
	}

	for _, providerName := range providerNames {
		var svc brokerapi.Service
		if providerName == "TENANT" {
			svc = sharedService
		} else {

			provider, err := client.GetProvider(providerName)
			if err != nil {
				return services, err
			}

			svc = service(provider)
		}

		whitelistedPlans, isWhitelisted := b.whitelist[providerName]
		if b.whitelist == nil || isWhitelisted {
			if isWhitelisted {
				svc = applyWhitelist(svc, whitelistedPlans)
			}
			services = append(services, svc)
		}
	}

	return services, nil
}

func service(provider *atlas.Provider) (service brokerapi.Service) {
	// Create a CLI-friendly and user-friendly name. Will be displayed in the
	// marketplace generated by the service catalog.
	catalogName := fmt.Sprintf("mongodb-atlas-%s", strings.ToLower(provider.Name))

	service = brokerapi.Service{
		ID:                   serviceIDForProvider(provider),
		Name:                 catalogName,
		Description:          fmt.Sprintf(`Atlas cluster hosted on "%s"`, provider.Name),
		Bindable:             true,
		InstancesRetrievable: false,
		BindingsRetrievable:  false,
		Metadata:             nil,
		PlanUpdatable:        true,
		Plans:                plansForProvider(provider),
	}

	return service
}

func findProviderByServiceID(client atlas.Client, serviceID string) (*atlas.Provider, error) {
	for _, providerName := range providerNames {
		provider, err := client.GetProvider(providerName)
		if err != nil {
			return nil, err
		}

		if serviceIDForProvider(provider) == serviceID {
			return provider, nil
		}
	}

	return nil, apiresponses.NewFailureResponse(errors.New("Invalid service ID"), http.StatusBadRequest, "invalid-service-id")
}

func findInstanceSizeByPlanID(provider *atlas.Provider, planID string) (*atlas.InstanceSize, error) {
	for _, instanceSize := range provider.InstanceSizes {
		if planIDForInstanceSize(provider, instanceSize) == planID {
			return &instanceSize, nil
		}
	}

	return nil, apiresponses.NewFailureResponse(errors.New("Invalid plan ID"), http.StatusBadRequest, "invalid-plan-id")
}

// plansForProvider will convert the available instance sizes for a provider
// to service plans for the broker.
func plansForProvider(provider *atlas.Provider) []brokerapi.ServicePlan {
	var plans []brokerapi.ServicePlan

	for _, instanceSize := range provider.InstanceSizes {
		plan := brokerapi.ServicePlan{
			ID:          planIDForInstanceSize(provider, instanceSize),
			Name:        instanceSize.Name,
			Description: fmt.Sprintf("Instance size \"%s\"", instanceSize.Name),
		}

		plans = append(plans, plan)
	}

	return plans
}

// serviceIDForProvider will generate a globally unique ID for a provider.
func serviceIDForProvider(provider *atlas.Provider) string {
	return fmt.Sprintf("%s-service-%s", idPrefix, strings.ToLower(provider.Name))
}

// planIDForInstanceSize will generate a globally unique ID for an instance size
// on a specific provider.
func planIDForInstanceSize(provider *atlas.Provider, instanceSize atlas.InstanceSize) string {
	return fmt.Sprintf("%s-plan-%s-%s", idPrefix, strings.ToLower(provider.Name), strings.ToLower(instanceSize.Name))
}
