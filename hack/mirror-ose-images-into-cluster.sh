set -x
set -e

ROOT_DIR=$(dirname "${BASH_SOURCE}")/..

REPO_NAMESPACE="${REPO_NAMESPACE:-"openshift"}"
OSE_IMAGE_TAG="${OSE_IMAGE_TAG:-"v4.2"}"
SOURCE_IMAGE_URL="${SOURCE_IMAGE_URL:-"registry-proxy.engineering.redhat.com"}"
SOURCE_IMAGE_NAMESPACE="${SOURCE_IMAGE_NAMESPACE:-"rh-osbs"}"
OUTPUT_IMAGE_URL="${OUTPUT_IMAGE_URL:-"$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')"}"
OUTPUT_IMAGE_NAMESPACE="${OUTPUT_IMAGE_NAMESPACE:-"$REPO_NAMESPACE"}"
SETUP_REGISTRY_AUTH="${SETUP_REGISTRY_AUTH:-"true"}"
PULL_IMAGES="${PULL_IMAGES:-"true"}"
PUSH_IMAGES="${PUSH_IMAGES:-"true"}"

if [ -z "$OUTPUT_IMAGE_URL" ]; then
    echo "Couldn't detect \$OUTPUT_IMAGE_URL or unset"
    exit 1
fi

# default to mirroring the $OSE_IMAGE_TAG for each, but allow overriding the tag to be mirrored per mirror
METERING_ANSIBLE_OPERATOR_IMAGE_TAG="${METERING_ANSIBLE_OPERATOR_IMAGE_TAG:-$OSE_IMAGE_TAG}"
METERING_REPORTING_OPERATOR_IMAGE_TAG="${METERING_REPORTING_OPERATOR_IMAGE_TAG:-$OSE_IMAGE_TAG}"
METERING_PRESTO_IMAGE_TAG="${METERING_PRESTO_IMAGE_TAG:-$OSE_IMAGE_TAG}"
METERING_HIVE_IMAGE_TAG="${METERING_HIVE_IMAGE_TAG:-$OSE_IMAGE_TAG}"
METERING_HADOOP_IMAGE_TAG="${METERING_HADOOP_IMAGE_TAG:-$OSE_IMAGE_TAG}"
GHOSTUNNEL_IMAGE_TAG="${GHOSTUNNEL_IMAGE_TAG:-$OSE_IMAGE_TAG}"
OAUTH_PROXY_IMAGE_TAG="${OAUTH_PROXY_IMAGE_TAG:-$OSE_IMAGE_TAG}"

METERING_ANSIBLE_OPERATOR_SOURCE_IMAGE="${METERING_ANSIBLE_OPERATOR_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-metering-ansible-operator:$METERING_ANSIBLE_OPERATOR_IMAGE_TAG"}"
METERING_REPORTING_OPERATOR_SOURCE_IMAGE="${METERING_REPORTING_OPERATOR_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-metering-reporting-operator:$METERING_REPORTING_OPERATOR_IMAGE_TAG"}"
METERING_PRESTO_SOURCE_IMAGE="${METERING_PRESTO_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-metering-presto:$METERING_PRESTO_IMAGE_TAG"}"
METERING_HIVE_SOURCE_IMAGE="${METERING_HIVE_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-metering-hive:$METERING_HIVE_IMAGE_TAG"}"
METERING_HADOOP_SOURCE_IMAGE="${METERING_HADOOP_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-metering-hadoop:$METERING_HADOOP_IMAGE_TAG"}"
GHOSTUNNEL_SOURCE_IMAGE="${GHOSTUNNEL_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-ghostunnel:$GHOSTUNNEL_IMAGE_TAG"}"
OAUTH_PROXY_SOURCE_IMAGE="${OAUTH_PROXY_SOURCE_IMAGE:-"$SOURCE_IMAGE_NAMESPACE/openshift-ose-oauth-proxy:$OAUTH_PROXY_IMAGE_TAG"}"

METERING_ANSIBLE_OPERATOR_OUTPUT_IMAGE="${METERING_ANSIBLE_OPERATOR_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-metering-ansible-operator:$METERING_ANSIBLE_OPERATOR_IMAGE_TAG"}"
METERING_REPORTING_OPERATOR_OUTPUT_IMAGE="${METERING_REPORTING_OPERATOR_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-metering-reporting-operator:$METERING_REPORTING_OPERATOR_IMAGE_TAG"}"
METERING_PRESTO_OUTPUT_IMAGE="${METERING_PRESTO_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-metering-presto:$METERING_PRESTO_IMAGE_TAG"}"
METERING_HIVE_OUTPUT_IMAGE="${METERING_HIVE_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-metering-hive:$METERING_HIVE_IMAGE_TAG"}"
METERING_HADOOP_OUTPUT_IMAGE="${METERING_HADOOP_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-metering-hadoop:$METERING_HADOOP_IMAGE_TAG"}"
GHOSTUNNEL_OUTPUT_IMAGE="${GHOSTUNNEL_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-ghostunnel:$GHOSTUNNEL_IMAGE_TAG"}"
OAUTH_PROXY_OUTPUT_IMAGE="${OAUTH_PROXY_OUTPUT_IMAGE:-"$OUTPUT_IMAGE_NAMESPACE/ose-oauth-proxy:$OAUTH_PROXY_IMAGE_TAG"}"

: "${METERING_NAMESPACE:?"\$METERING_NAMESPACE must be set!"}"

if [ "$SETUP_REGISTRY_AUTH" == "true" ]; then
    echo "Creating namespace for images: $REPO_NAMESPACE"
    oc create namespace "$REPO_NAMESPACE" || true
    echo "Creating serviceaccount registry-editor in $REPO_NAMESPACE"
    oc create serviceaccount registry-editor -n "$REPO_NAMESPACE" || true
    echo "Granting registry-editor registry-editor permissions in $REPO_NAMESPACE"
    oc adm policy add-role-to-user registry-editor -z registry-editor -n "$REPO_NAMESPACE" || true
    echo "Performing docker login as registry-editor to $OUTPUT_IMAGE_URL"
    set +x
    docker login \
        "$OUTPUT_IMAGE_URL" \
        -u registry-editor \
        -p "$(oc sa get-token registry-editor -n "$REPO_NAMESPACE")"
    set -x
fi

echo "Ensuring namespace $REPO_NAMESPACE exists for images to be pushed into"
oc create namespace "$REPO_NAMESPACE" || true
echo "Pushing Metering OSE images to $OUTPUT_IMAGE_URL"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$METERING_ANSIBLE_OPERATOR_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$METERING_ANSIBLE_OPERATOR_OUTPUT_IMAGE"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$METERING_REPORTING_OPERATOR_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$METERING_REPORTING_OPERATOR_OUTPUT_IMAGE"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$METERING_PRESTO_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$METERING_PRESTO_OUTPUT_IMAGE"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$METERING_HIVE_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$METERING_HIVE_OUTPUT_IMAGE"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$METERING_HADOOP_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$METERING_HADOOP_OUTPUT_IMAGE"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$GHOSTUNNEL_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$GHOSTUNNEL_OUTPUT_IMAGE"

"$ROOT_DIR/hack/mirror-ose-image.sh" \
    "$SOURCE_IMAGE_URL/$OAUTH_PROXY_SOURCE_IMAGE" \
    "$OUTPUT_IMAGE_URL/$OAUTH_PROXY_OUTPUT_IMAGE"

echo "Granting access to pull images in $REPO_NAMESPACE to all serviceaccounts in \$METERING_NAMESPACE=$METERING_NAMESPACE"
oc -n "$REPO_NAMESPACE" policy add-role-to-group system:image-puller "system:serviceaccounts:$METERING_NAMESPACE" --rolebinding-name "$METERING_NAMESPACE-image-pullers"
