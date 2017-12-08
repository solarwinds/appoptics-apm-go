#!/bin/bash
# build dot graph files for specified tests and open as PDFs
# runs go test $@ in current dir, e.g.:
# ./test_graphviz.sh -v
# ./test_graphviz.sh -v -tags disable_tracing
# ./test_graphviz.sh -v -run TestTraceHTTP
# ./test_graphviz.sh -v -tags traceview github.com/librato/go-traceview/v1/tv/internal/traceview/
graphdir="${DOT_GRAPHDIR:=$(pwd)}"
DOT_GRAPHS=1 DOT_GRAPHDIR="$graphdir" go test "$@"
OPEN="echo"
if [ "$(uname)" == "Darwin" ] && [ -t 1 ]; then # open if interactive mac shell
    OPEN="sleep 2; open" # seems to avoid Preview.app permission error
fi
all=""
for i in $graphdir/*.dot; do
    outf="${i%.dot}.pdf"
    # draw graph for any new DOT files
    if [ ! -f "$outf" ]; then
        echo "GRAPHVIZ $outf"
        dot -Tpdf $i -o $outf 2>&1 >/dev/null | grep -v CGFontGetGlyph
        all+=" $outf"
    fi
done
eval $OPEN $all
