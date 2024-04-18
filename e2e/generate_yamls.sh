#!/bin/sh

if [ ! -d yamls ]; then
	mkdir yamls
fi

# specify CNI version (default: 0.4.0)
export CNI_VERSION=${CNI_VERSION:-0.4.0}

templates_dir="$(dirname $(readlink -f $0))/templates"

# generate yaml files based on templates/*.j2 to yamls directory
for i in `ls templates/`; do
	echo $i
	gomplate -o yamls/${i%.j2} -f ${templates_dir}/$i
done
unset CNI_VERSION
