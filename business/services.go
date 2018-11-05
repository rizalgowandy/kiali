package business

import (
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/prometheus"
	"github.com/kiali/kiali/prometheus/internalmetrics"
)

// SvcService deals with fetching istio/kubernetes services related content and convert to kiali model
type SvcService struct {
	prom          prometheus.ClientInterface
	k8s           kubernetes.IstioClientInterface
	businessLayer *Layer
}

// GetServiceList returns a list of all services for a given Namespace
func (in *SvcService) GetServiceList(namespace string) (*models.ServiceList, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "SvcService", "GetServiceList")
	defer promtimer.ObserveNow(&err)

	var svcs []v1.Service
	var pods []v1.Pod

	wg := sync.WaitGroup{}
	wg.Add(2)
	errChan := make(chan error, 2)

	go func() {
		defer wg.Done()
		var err2 error
		svcs, err2 = in.k8s.GetServices(namespace, nil)
		if err2 != nil {
			log.Errorf("Error fetching Services per namespace %s: %s", namespace, err2)
			errChan <- err2
		}
	}()

	go func() {
		defer wg.Done()
		var err2 error
		pods, err2 = in.k8s.GetPods(namespace, "")
		if err2 != nil {
			log.Errorf("Error fetching Pods per namespace %s: %s", namespace, err2)
			errChan <- err2
		}
	}()

	wg.Wait()
	if len(errChan) != 0 {
		err = <-errChan
		return nil, err
	}

	// Convert to Kiali model
	return in.buildServiceList(models.Namespace{Name: namespace}, svcs, pods), nil
}

func (in *SvcService) buildServiceList(namespace models.Namespace, svcs []v1.Service, pods []v1.Pod) *models.ServiceList {
	services := make([]models.ServiceOverview, len(svcs))
	conf := config.Get()
	// Convert each k8s service into our model
	for i, item := range svcs {
		sPods := kubernetes.FilterPodsForService(&item, pods)
		/** Check if Service has istioSidecar deployed */
		mPods := models.Pods{}
		mPods.Parse(sPods)
		hasSideCar := mPods.HasIstioSideCar()
		/** Check if Service has the label app required by Istio */
		_, appLabel := item.Spec.Selector[conf.IstioLabels.AppLabelName]
		services[i] = models.ServiceOverview{
			Name:         item.Name,
			IstioSidecar: hasSideCar,
			AppLabel:     appLabel,
		}
	}

	return &models.ServiceList{Namespace: namespace, Services: services}
}

// GetService returns a single service
func (in *SvcService) GetService(namespace, service, interval string, queryTime time.Time) (*models.ServiceDetails, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "SvcService", "GetService")
	defer promtimer.ObserveNow(&err)

	var svc *v1.Service
	var eps *v1.Endpoints

	wg := sync.WaitGroup{}
	wg.Add(2)
	errChan := make(chan error, 2)

	go func() {
		defer wg.Done()
		var err2 error
		svc, err2 = in.k8s.GetService(namespace, service)
		if err2 != nil {
			log.Errorf("Error fetching Service per namespace %s and service %s: %s", namespace, service, err2)
			errChan <- err2
		}
	}()

	go func() {
		defer wg.Done()
		var err2 error
		eps, err2 = in.k8s.GetEndpoints(namespace, service)
		if err2 != nil {
			log.Errorf("Error fetching Endpoints per namespace %s and service %s: %s", namespace, service, err2)
			errChan <- err2
		}
	}()

	wg.Wait()
	if len(errChan) != 0 {
		err = <-errChan
		return nil, err
	}

	var pods []v1.Pod
	var hth models.ServiceHealth
	var vs, dr []kubernetes.IstioObject
	var sWk map[string][]prometheus.Workload
	var ws models.Workloads

	wg = sync.WaitGroup{}
	wg.Add(8)
	errChan = make(chan error, 6)

	labelsSelector := labels.Set(svc.Spec.Selector).String()

	go func() {
		defer wg.Done()
		var err2 error
		pods, err2 = in.k8s.GetPods(namespace, labelsSelector)
		if err2 != nil {
			errChan <- err2
		}
	}()

	go func() {
		defer wg.Done()
		hth = in.businessLayer.Health.getServiceHealth(namespace, service, interval, queryTime, svc)
	}()

	go func() {
		defer wg.Done()
		var err2 error
		vs, err2 = in.k8s.GetVirtualServices(namespace, service)
		if err2 != nil {
			errChan <- err2
		}
	}()

	go func() {
		defer wg.Done()
		var err2 error
		dr, err2 = in.k8s.GetDestinationRules(namespace, service)
		if err2 != nil {
			errChan <- err2
		}
	}()

	go func() {
		defer wg.Done()
		var err2 error
		ns, err2 := in.businessLayer.Namespace.GetNamespace(namespace)
		if err2 != nil {
			log.Errorf("Error fetching details of namespace %s: %s", namespace, err2)
			errChan <- err2
		}

		sWk, err2 = in.prom.GetSourceWorkloads(ns.Name, ns.CreationTimestamp, service)
		if err2 != nil {
			log.Errorf("Error fetching SourceWorkloads per namespace %s and service %s: %s", namespace, service, err2)
			errChan <- err2
		}
	}()

	go func() {
		defer wg.Done()
		var err2 error
		ws, err2 = fetchWorkloads(in.k8s, namespace, labelsSelector)
		if err2 != nil {
			log.Errorf("Error fetching Workloads per namespace %s and service %s: %s", namespace, service, err2)
			errChan <- err2
		}
	}()

	var vsUpdate, vsDelete bool
	go func() {
		defer wg.Done()
		vsUpdate, vsDelete = getUpdateDeletePermissions(in.k8s, namespace, VirtualServices)
	}()

	var drUpdate, drDelete bool
	go func() {
		defer wg.Done()
		drUpdate, drDelete = getUpdateDeletePermissions(in.k8s, namespace, DestinationRules)
	}()

	wg.Wait()
	if len(errChan) != 0 {
		err = <-errChan
		return nil, err
	}

	wo := models.WorkloadOverviews{}
	for _, w := range ws {
		wi := &models.WorkloadListItem{}
		wi.ParseWorkload(w)
		wo = append(wo, wi)
	}

	s := models.ServiceDetails{Workloads: wo, Health: hth}
	s.SetService(svc)
	s.SetPods(kubernetes.FilterPodsForEndpoints(eps, pods))
	s.SetEndpoints(eps)
	s.SetVirtualServices(vs, vsUpdate, vsDelete)
	s.SetDestinationRules(dr, drUpdate, drDelete)
	s.SetSourceWorkloads(sWk)
	return &s, nil
}
