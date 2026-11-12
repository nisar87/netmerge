module netmerge

go 1.24

require netmerge/engine v0.0.0

// NetMerge's own network engine lives in engine/ as a separate module; NetMerge.exe can run it (-engine).
replace netmerge/engine => ./engine
