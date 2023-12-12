package simconsul

import (
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/nomad/client/consul"
	"github.com/hashicorp/nomad/client/serviceregistration"
)

type NoopCatalogAPI struct{}

func (NoopCatalogAPI) Datacenters() ([]string, error) {
	return []string{}, nil
}

func (NoopCatalogAPI) Service(service string, tag string, q *api.QueryOptions) ([]*api.CatalogService, *api.QueryMeta, error) {
	return []*api.CatalogService{}, &api.QueryMeta{}, nil
}

// NoopSupportedProxiesAPI implements github.com/hashicorp/nomad/client/consul.SupportedProxiesAPI
type NoopSupportedProxiesAPI struct{}

func (NoopSupportedProxiesAPI) Proxies() (map[string][]string, error) {
	return map[string][]string{}, nil
}

type NOOPSupportedProxiesAPIFunc func(string) consul.SupportedProxiesAPI

// NoopServiceRegHandler implements github.com/hashicorp/nomad/client/serviceregistration.Handler
type NoopServiceRegHandler struct{}

func (NoopServiceRegHandler) RegisterWorkload(workload *serviceregistration.WorkloadServices) error {
	return nil
}
func (NoopServiceRegHandler) RemoveWorkload(workload *serviceregistration.WorkloadServices) {}
func (NoopServiceRegHandler) UpdateWorkload(old, newTask *serviceregistration.WorkloadServices) error {
	return nil
}
func (NoopServiceRegHandler) AllocRegistrations(allocID string) (*serviceregistration.AllocRegistration, error) {
	return nil, nil
}
func (NoopServiceRegHandler) UpdateTTL(id, namespace, output, status string) error { return nil }
