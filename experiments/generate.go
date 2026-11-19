package experiments

// One target per experiment: go generate -run <name> ./experiments reruns
// one, go generate ./experiments every one. The results go to data/;
// tools/explainer/build.sh then rebuilds the economy page from them.

//go:generate go run ./run -out data base
//go:generate go run ./run -out data strike
//go:generate go run ./run -out data blight
//go:generate go run ./run -out data trade
//go:generate go run ./run -out data boom
//go:generate go run ./run -out data corridor
//go:generate go run ./run -out data aa
//go:generate go run ./run -out data recipes
//go:generate go run ./run -out data sweep
//go:generate go run ./run -out data money
//go:generate go run ./run -out data cornlaws
//go:generate go run ./run -out data brexit
