package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0 rbac:roleName=gari paths="./..."
//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0 crd object paths="./apis/..."
//go:generate go run k8s.io/code-generator/cmd/register-gen@v0.28.1 --input-dirs ./apis/v1alpha1  --output-package apis/v1alpha1 --go-header-file tools/boilerplate/boilerplate.go.txt
