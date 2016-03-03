#!/bin/bash
# build dot graph files for specified tests and open

DOT_GRAPHS=1 go test "$@"
all=""
for i in *.dot; do
    if [ ! -f "$i.pdf" ]; then
	echo "GRAPHVIZ $i.pdf"
	dot -Tpdf $i -o $i.pdf 2>&1 >/dev/null | grep -v CGFontGetGlyph
	all+=" $i.pdf"
    fi
done
open $all
