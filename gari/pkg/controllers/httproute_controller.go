package controllers

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/gari/pkg/gateway"
	"sigs.k8s.io/kubebuilder-declarative-pattern/commonclient"
	// "sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon"
)

var _ reconcile.Reconciler = &HTTPRouteController{}

// HTTPRouteController reconciles a HTTPRoute object
type HTTPRouteController struct {
	client client.Client
	mgr    ctrl.Manager

	finalizer string

	// watchsets *watchset.Manager
	// // Log    logr.Logger
	// // Scheme *runtime.Scheme

	Gateway *gateway.Instance
	// controller controller.Controller
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch

func (r *HTTPRouteController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	//log := klog.FromContext(ctx)

	id := req.NamespacedName
	route := &gatewayapi.HTTPRoute{}
	if err := r.client.Get(ctx, id, route); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !route.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.Gateway.DeleteHTTPRoute(ctx, route); err != nil {
			return ctrl.Result{}, err
		}

		// remove our finalizer from the list and update it.
		if changed := controllerutil.RemoveFinalizer(route, r.finalizer); changed {
			if err := r.client.Update(ctx, route); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	if added := controllerutil.AddFinalizer(route, r.finalizer); added {
		if err := r.client.Update(ctx, route); err != nil {
			return reconcile.Result{}, err
		}
	}

	if err := r.Gateway.UpdateHTTPRoute(ctx, route); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteController) SetupWithManager(mgr ctrl.Manager) error {
	// addon.Init()

	r.finalizer = "HTTPRouteController"

	r.client = mgr.GetClient()
	r.mgr = mgr

	// labels := map[string]string{
	// 	"k8s-app": "compositedefinition",
	// }

	// watchLabels := declarative.SourceLabel(mgr.GetScheme())

	// if err := r.Reconciler.Init(mgr, &addonsv1alpha1.CompositeDefinition{},
	// 	declarative.WithObjectTransform(declarative.AddLabels(labels)),
	// 	declarative.WithOwner(declarative.SourceAsOwner),
	// 	declarative.WithLabels(watchLabels),
	// 	declarative.WithStatus(status.NewBasic(mgr.GetClient())),
	// 	// TODO: add an application to your manifest:  declarative.WithObjectTransform(addon.TransformApplicationFromStatus),
	// 	// TODO: add an application to your manifest:  declarative.WithManagedApplication(watchLabels),
	// 	declarative.WithObjectTransform(addon.ApplyPatches),
	// ); err != nil {
	// 	return err
	// }

	c, err := controller.New("httproute-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to HTTPRoute
	err = c.Watch(commonclient.SourceKind(mgr.GetCache(), &gatewayapi.HTTPRoute{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to deployed objects
	// _, err = declarative.WatchChildren(declarative.WatchChildrenOptions{Manager: mgr, Controller: c, Reconciler: r, LabelMaker: watchLabels})
	// if err != nil {
	// 	return err
	// }

	return nil
}
