#!/bin/sh

if [ ! -d yamls ]; then
	mkdir yamls
fi

# specify CNI version (default: 0.4.0)
CNI_VERSION=${CNI_VERSION:-0.4.0}

templates_dir="$(dirname $(readlink -f $0))/templates"

# generate yaml files based on templates/*.j2 to yamls directory
for i in `ls ${templates_dir}/*.j2`; do
	echo "Processing $i..."
	# Use sed to replace the placeholder with the CNI_VERSION variable
	sed "s/{{ CNI_VERSION }}/$CNI_VERSION/g" $i > yamls/$(basename ${i%.j2})
done
