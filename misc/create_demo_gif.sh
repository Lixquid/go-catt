pushd ..
go build
popd
podman run --rm -v "$PWD/../catt:/usr/local/bin/catt:z" -v $PWD:/vhs ghcr.io/charmbracelet/vhs demo.tape
