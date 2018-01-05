#!/bin/bash -e

OUTFILE=$1
OUTPUT_DIR_NAME="${OUTFILE%%.zip}"
TMPDIR="$(mktemp -d)"
OUTPUT_DIR="$TMPDIR/$OUTPUT_DIR_NAME"

mkdir -p "$OUTPUT_DIR"

mkdir -p "$OUTPUT_DIR/hack"
cp \
    hack/alm-install.sh \
    hack/alm-uninstall.sh \
    hack/util.sh \
    hack/default-env.sh \
    "$OUTPUT_DIR/hack/"

mkdir -p "$OUTPUT_DIR/Documentation"
cp \
    Documentation/install-chargeback.md \
    Documentation/report.md \
    Documentation/using-chargeback.md \
    Documentation/chargeback-config.md \
    "$OUTPUT_DIR/Documentation/"

mkdir -p "$OUTPUT_DIR/manifests"
cp -r \
    manifests/custom-resources \
    "$OUTPUT_DIR/manifests/"
cp -r \
    manifests/custom-resource-definitions \
    "$OUTPUT_DIR/manifests/"

# Remove scheduled reports folder since we currently do not support them
rm -r "$OUTPUT_DIR/manifests/custom-resources/scheduled-reports"

cp -r \
    manifests/installer \
    "$OUTPUT_DIR/manifests/"

cp -r \
    manifests/alm \
    "$OUTPUT_DIR/manifests/"

mkdir -p $OUTPUT_DIR/manifests/chargeback-config
cp \
    manifests/chargeback-config/custom-values.yaml \
    "$OUTPUT_DIR/manifests/chargeback-config"
echo "Start with Documentation/install-chargeback.md" > "$OUTPUT_DIR/README"


pushd "$TMPDIR"
zip -r "$OUTFILE" "$OUTPUT_DIR_NAME"
popd

mv "$OUTPUT_DIR.zip" .
