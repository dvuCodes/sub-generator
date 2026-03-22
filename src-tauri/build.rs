use std::{
    env,
    fs,
    path::{Path, PathBuf},
    process::Command,
};

fn main() {
    let manifest_dir =
        PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR not set"));
    let workspace_dir = manifest_dir
        .parent()
        .expect("src-tauri should live under the workspace root");
    let go_sidecar_dir = workspace_dir.join("go-sidecar");

    emit_rerun_markers(&go_sidecar_dir);
    build_go_sidecar(&manifest_dir, &go_sidecar_dir);

    tauri_build::build()
}

fn emit_rerun_markers(go_sidecar_dir: &Path) {
    println!("cargo:rerun-if-changed={}", go_sidecar_dir.display());
}

fn build_go_sidecar(manifest_dir: &Path, go_sidecar_dir: &Path) {
    let target = env::var("TARGET").expect("TARGET not set");
    let output_name = if target.contains("windows") {
        format!("subgen-sidecar-{target}.exe")
    } else {
        format!("subgen-sidecar-{target}")
    };
    let output_path = manifest_dir.join(output_name);

    let status = Command::new("go")
        .current_dir(go_sidecar_dir)
        .args(["build", "-o"])
        .arg(&output_path)
        .arg(".")
        .status()
        .expect("failed to invoke go build for the sidecar");

    if !status.success() {
        panic!("go build failed for sidecar at {}", output_path.display());
    }

    // Tauri copies the external bin during its build; removing any stale debug copy
    // prevents it from reusing an older sidecar if the external-bin copy step is skipped.
    let stale_debug_binary = manifest_dir.join("target").join("debug").join(if target.contains("windows") {
        "subgen-sidecar.exe"
    } else {
        "subgen-sidecar"
    });
    if stale_debug_binary.exists() {
        let _ = fs::remove_file(stale_debug_binary);
    }
}
