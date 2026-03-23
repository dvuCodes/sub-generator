use std::{
    env, fs,
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

    sync_dev_sidecar_copies(manifest_dir, &output_path, target.contains("windows"));
}

fn sync_dev_sidecar_copies(manifest_dir: &Path, source_path: &Path, is_windows: bool) {
    let sidecar_name = if is_windows {
        "subgen-sidecar.exe"
    } else {
        "subgen-sidecar"
    };
    let copy_targets = [
        manifest_dir.join("target").join("debug").join(sidecar_name),
        manifest_dir
            .join("target")
            .join("dev-tauri")
            .join("debug")
            .join(sidecar_name),
    ];

    for destination in copy_targets {
        if let Some(parent) = destination.parent() {
            let _ = fs::create_dir_all(parent);
        }
        fs::copy(source_path, &destination).unwrap_or_else(|err| {
            panic!(
                "failed to sync sidecar from {} to {}: {}",
                source_path.display(),
                destination.display(),
                err
            )
        });
    }
}
