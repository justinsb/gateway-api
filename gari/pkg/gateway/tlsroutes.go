package gateway

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	// kinspire "github.com/justinsb/packages/kinspire/client"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

type tlsRoutes struct {
	// spiffeID string
	// spiffe   *kinspire.SPIFFESource
	mutex  sync.RWMutex
	byID   map[types.NamespacedName]*tlsRoute
	byHost map[string][]*tlsRoute
}

func (r *tlsRoutes) init() {
	r.byID = make(map[types.NamespacedName]*tlsRoute)
	r.byHost = make(map[string][]*tlsRoute)
}

// Should be immutable
type tlsRoute struct {
	id    types.NamespacedName
	hosts []string
	// spiffeID string
	// spiffe   *kinspire.SPIFFESource
	obj gatewayapi.TLSRoute
}

func (r *tlsRoutes) lookupTLSRoute(ctx context.Context, host string) (tlsRouteMatch, bool) {
	r.mutex.RLock()
	tlsRoutes := r.byHost[host]
	r.mutex.RUnlock()

	var bestMatch tlsRouteMatch
	bestMatch.score = math.MinInt
	for _, tlsRoute := range tlsRoutes {
		tlsRouteMatch, found := tlsRoute.matches(host)
		if found && tlsRouteMatch.score > bestMatch.score {
			bestMatch = tlsRouteMatch
		}
	}

	if bestMatch.score == math.MinInt {
		return tlsRouteMatch{}, false
	}

	return bestMatch, true
}

type tlsRouteMatch struct {
	score int
	route *tlsRoute
}

func (r *tlsRoute) matches(host string) (tlsRouteMatch, bool) {
	var bestMatch tlsRouteMatch
	bestMatch.score = math.MinInt

	for _, hostname := range r.hosts {
		// TODO: handle wildcard hostnames
		if hostname == host {
			bestMatch = tlsRouteMatch{
				score: 1,
				route: r,
			}
		}
	}

	if bestMatch.score == math.MinInt {
		return tlsRouteMatch{}, false
	}
	return bestMatch, true
}

func (r *tlsRoutes) UpdateTLSRoute(ctx context.Context, client client.Client, route *gatewayapi.TLSRoute) error {
	id := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
	return r.updateTLSRoute(id, route)
}

func (r *tlsRoutes) DeleteTLSRoute(ctx context.Context, client client.Client, route *gatewayapi.TLSRoute) error {
	id := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
	return r.updateTLSRoute(id, nil)
}

func (r *tlsRoutes) updateTLSRoute(id types.NamespacedName, newObj *gatewayapi.TLSRoute) error {
	var newTLSRoute *tlsRoute
	if newObj != nil {
		var hosts []string
		for _, hostname := range newObj.Spec.Hostnames {
			hosts = append(hosts, string(hostname))
		}

		newTLSRoute = &tlsRoute{
			id:    id,
			hosts: hosts,
			// spiffeID: r.spiffeID,
			// spiffe:   r.spiffe,
			obj: *newObj.DeepCopy(),
		}

		// for i := range newObj.Spec.Rules {
		// 	rule := &newObj.Spec.Rules[i]
		// 	hr := r.buildHTTPRule(ctx, client, id.Namespace, rule)
		// 	newTLSRoute.rules = append(newTLSRoute.rules, hr)
		// }
	}

	r.mutex.Lock()

	oldHTTPRoute := r.byID[id]
	if newTLSRoute == nil {
		delete(r.byID, id)
	} else {
		r.byID[id] = newTLSRoute
	}

	if oldHTTPRoute != nil {
		for _, host := range oldHTTPRoute.hosts {
			keepRoutes := filter(r.byHost[host], func(r *tlsRoute) bool {
				return r.id != id
			})

			if len(keepRoutes) == 0 {
				delete(r.byHost, host)
			} else {
				r.byHost[host] = keepRoutes
			}
		}
	}

	if newTLSRoute != nil {
		for _, host := range newTLSRoute.hosts {
			r.byHost[host] = append(r.byHost[host], newTLSRoute)
			klog.Infof("added TLSRoute for host %q", host)
		}
	}

	r.mutex.Unlock()

	return nil
}

func (m *tlsRouteMatch) serveTLS(ctx context.Context, clientConn net.Conn, clientHello *clientHelloInfo) error {
	log := klog.FromContext(ctx)

	tlsRoute := &m.route.obj
	if len(tlsRoute.Spec.Rules) != 1 {
		return fmt.Errorf("expected 1 rule in TLSRoute, got %d", len(m.route.obj.Spec.Rules))
	}
	rule := &m.route.obj.Spec.Rules[0]
	backendRefs := rule.BackendRefs
	if len(backendRefs) == 0 {
		return fmt.Errorf("no backendRefs in rule")
	}

	// TODO: Better load balancing etc
	backendRef := backendRefs[0]

	// TODO: Go direct to endpoints?
	serviceName := string(backendRef.Name)
	serviceNamespace := "" // TODO: backendRef.Namespace
	if serviceNamespace == "" {
		serviceNamespace = tlsRoute.Namespace
	}
	backendHostName := serviceName + "." + serviceNamespace
	backendPort := gatewayv1.PortNumber(0)
	if backendRef.Port != nil {
		backendPort = *(backendRef.Port)
	}
	if backendPort == 0 {
		return fmt.Errorf("cannot infer backendRef port")
	}

	log.Info("dialing backend", "backendHostName", backendHostName, "backendPort", backendPort)
	dest := net.JoinHostPort(backendHostName, strconv.Itoa(int(backendPort)))
	backendConn, err := net.DialTimeout("tcp", dest, 2*time.Second)
	if err != nil {
		return fmt.Errorf("error dialing backend: %w", err)
	}
	defer backendConn.Close()

	log.Info("dialed backend", "dest", dest)

	if err := backendConn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return fmt.Errorf("error setting write deadline: %w", err)
	}

	if _, err := backendConn.Write(clientHello.Header); err != nil {
		return fmt.Errorf("error writing client hello to backend: %w", err)
	}
	if _, err := backendConn.Write(clientHello.Message); err != nil {
		return fmt.Errorf("error writing client hello to backend: %w", err)
	}

	if err := backendConn.SetWriteDeadline(time.Time{}); err != nil {
		return fmt.Errorf("error clearing write deadline: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		io.Copy(clientConn, backendConn)
		clientConn.(*net.TCPConn).CloseWrite()
		wg.Done()
	}()
	go func() {
		io.Copy(backendConn, clientConn)
		backendConn.(*net.TCPConn).CloseWrite()
		wg.Done()
	}()

	wg.Wait()
	log.Info("closed connections")
	return nil
}
