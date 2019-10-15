#!/bin/bash

# kubectl -n openshift-marketplace port-forward svc/art-applications 50051

read -r -d '' JQ_PROGRAM << EOM
 .spec.install.spec.deployments[0].spec.template.spec.containers[]
 | select(.name=="operator")
 | .env
 | map(select(.name | match(".*_IMAGE")))
 # | map(.value |= ltrimstr("image-registry.openshift-image-registry.svc:5000/"))
 # | map(.value |= (split(":"))[1])
 # |  map(.name |= gsub("_IMAGE"; "_IMAGE_TAG"))
 | .[]
 | "export " + .name + "=" + .value
EOM

grpcurl -plaintext -d '{"pkgName": "metering-ocp", "channelName": "4.2"}' localhost:50051 api.Registry/GetBundleForChannel  \
    | jq '.csvJson' -r \
    | jq "$JQ_PROGRAM" -r

#'.spec.install.spec.deployments[0].spec.template.spec.containers[] | select(.name=="operator") | .env | map(select(.name | match(".*_IMAGE"))) | map(.value |= ltrimstr("image-registry.openshift-image-registry.svc:5000/")) | map(.value |= (split("@sha256:"))[1]) | map(.value |= "@sha256:" + .) | map(.name |= gsub("_IMAGE"; "_IMAGE_TAG")) | .[] | "export " + .name + "=" + .value' -r
    # | jq '.spec.install.spec.deployments[0].spec.template.spec.containers[] | select(.name=="operator") | .env | map(select(.name | match(".*_IMAGE"))) | map(.value |= ltrimstr("image-registry.openshift-image-registry.svc:5000/")) | map(.value |= (split(":"))[1]) |  map(.name |= gsub("_IMAGE"; "_IMAGE_TAG")) | .[] | "export " + .name + "=" + .value' -r
