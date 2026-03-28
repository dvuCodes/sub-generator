use std::{
    env, fs, io,
    path::{Path, PathBuf},
    process::Command,
    time::SystemTime,
};

fn main() {
    configure_tauri_build_for_profile();

    let manifest_dir =
        PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR not set"));
    let workspace_dir = manifest_dir
        .parent()
        .expect("src-tauri should live under the workspace root");
    let go_sidecar_dir = workspace_dir.join("go-sidecar");
    let python_backend_source_dir = workspace_dir.join("python-backend");
    let ml_backend_dir = workspace_dir.join("services").join("ml-backend");

    emit_rerun_markers(&go_sidecar_dir, &python_backend_source_dir);
    build_go_sidecar(&manifest_dir, &go_sidecar_dir);
    sync_python_backend_assets(&python_backend_source_dir, &ml_backend_dir, &manifest_dir);
    override_tauri_resource_paths(&manifest_dir);

    tauri_build::build()
}

fn configure_tauri_build_for_profile() {
    if env::var("PROFILE").as_deref() != Ok("debug") {
        return;
    }

    let mut config_override = env::var("TAURI_CONFIG")
        .ok()
        .and_then(|value| serde_json::from_str::<serde_json::Value>(&value).ok())
        .unwrap_or_else(|| serde_json::json!({}));

    if !config_override.is_object() {
        config_override = serde_json::json!({});
    }

    let root = config_override
        .as_object_mut()
        .expect("config override should be an object");
    let bundle = root
        .entry("bundle")
        .or_insert_with(|| serde_json::json!({}));
    if !bundle.is_object() {
        *bundle = serde_json::json!({});
    }
    bundle["externalBin"] = serde_json::json!([]);
    bundle["resources"] = serde_json::json!([]);
    env::set_var("TAURI_CONFIG", config_override.to_string());
}

fn emit_rerun_markers(go_sidecar_dir: &Path, python_backend_source_dir: &Path) {
    println!("cargo:rerun-if-changed={}", go_sidecar_dir.display());
    println!("cargo:rerun-if-changed={}", python_backend_source_dir.display());
}

fn build_go_sidecar(manifest_dir: &Path, go_sidecar_dir: &Path) {
    let target = env::var("TARGET").expect("TARGET not set");
    let output_name = if target.contains("windows") {
        format!("subgen-sidecar-{target}.exe")
    } else {
        format!("subgen-sidecar-{target}")
    };
    let output_path = manifest_dir.join(output_name);

    if sidecar_rebuild_required(go_sidecar_dir, &output_path) {
        let status = Command::new("go")
            .current_dir(go_sidecar_dir)
            .args(["build", "-o"])
            .arg(&output_path)
            .arg(".")
            .status()
            .unwrap_or_else(|err| panic!("{}", format_go_build_error(&err)));

        if !status.success() {
            panic!("go build failed for sidecar at {}", output_path.display());
        }
    }

    sync_dev_sidecar_copies(manifest_dir, &output_path, target.contains("windows"));
}

fn sidecar_rebuild_required(go_sidecar_dir: &Path, output_path: &Path) -> bool {
    let output_modified = match fs::metadata(output_path).and_then(|metadata| metadata.modified()) {
        Ok(modified) => modified,
        Err(_) => return true,
    };

    newest_file_modified(go_sidecar_dir)
        .map(|modified| modified > output_modified)
        .unwrap_or(true)
}

fn newest_file_modified(path: &Path) -> io::Result<SystemTime> {
    let metadata = fs::metadata(path)?;
    if metadata.is_file() {
        return metadata.modified();
    }

    let mut newest = SystemTime::UNIX_EPOCH;
    for entry in fs::read_dir(path)? {
        let entry = entry?;
        let modified = newest_file_modified(&entry.path())?;
        if modified > newest {
            newest = modified;
        }
    }

    Ok(newest)
}

fn sync_python_backend_assets(
    source_dir: &Path,
    dev_target: &Path,
    manifest_dir: &Path,
) {
    if !source_dir.exists() {
        return;
    }
    let _ = sync_tree(source_dir, dev_target);

    if env::var("PROFILE").as_deref() == Ok("debug") {
        return;
    }

    let bundle_target = manifest_dir.join("resources").join("ml-backend");
    let _ = sync_tree(source_dir, &bundle_target);
}

fn override_tauri_resource_paths(manifest_dir: &Path) {
    if env::var("PROFILE").as_deref() == Ok("debug") {
        return;
    }

    let mut config_override = env::var("TAURI_CONFIG")
        .ok()
        .and_then(|value| serde_json::from_str::<serde_json::Value>(&value).ok())
        .unwrap_or_else(|| serde_json::json!({}));

    if !config_override.is_object() {
        config_override = serde_json::json!({});
    }

    let root = config_override
        .as_object_mut()
        .expect("config override should be an object");
    let bundle = root
        .entry("bundle")
        .or_insert_with(|| serde_json::json!({}));
    if !bundle.is_object() {
        *bundle = serde_json::json!({});
    }

    let resource_glob = manifest_dir
        .join("resources")
        .join("ml-backend")
        .join("**")
        .join("*")
        .display()
        .to_string();
    bundle["resources"] = serde_json::json!([resource_glob]);
    env::set_var("TAURI_CONFIG", config_override.to_string());
}

fn sync_tree(source: &Path, destination: &Path) -> io::Result<()> {
    if source.is_file() {
        if let Some(parent) = destination.parent() {
            fs::create_dir_all(parent)?;
        }
        if !file_copy_required(source, destination)? {
            return Ok(());
        }
        fs::copy(source, destination)?;
        return Ok(());
    }

    fs::create_dir_all(destination)?;
    for entry in fs::read_dir(source)? {
        let entry = entry?;
        let source_path = entry.path();
        let destination_path = destination.join(entry.file_name());
        sync_tree(&source_path, &destination_path)?;
    }
    Ok(())
}

fn file_copy_required(source: &Path, destination: &Path) -> io::Result<bool> {
    let source_metadata = fs::metadata(source)?;
    let destination_metadata = match fs::metadata(destination) {
        Ok(metadata) => metadata,
        Err(_) => return Ok(true),
    };

    if source_metadata.len() != destination_metadata.len() {
        return Ok(true);
    }

    let source_modified = source_metadata.modified()?;
    let destination_modified = destination_metadata.modified()?;
    Ok(source_modified > destination_modified)
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
    let source_metadata = fs::metadata(source_path).unwrap_or_else(|err| {
        panic!(
            "failed to read built sidecar {}: {}",
            source_path.display(),
            err
        )
    });
    let source_len = source_metadata.len();
    let source_modified = source_metadata.modified().ok();

    for destination in copy_targets {
        if !destination_needs_sync(&destination, source_len, source_modified) {
            continue;
        }
        if let Some(parent) = destination.parent() {
            let _ = fs::create_dir_all(parent);
        }
        if let Err(err) = fs::copy(source_path, &destination) {
            handle_sync_error(source_path, &destination, err);
        }
    }
}

fn destination_needs_sync(
    destination: &Path,
    source_len: u64,
    source_modified: Option<SystemTime>,
) -> bool {
    let destination_metadata = match fs::metadata(destination) {
        Ok(metadata) => metadata,
        Err(_) => return true,
    };

    if destination_metadata.len() != source_len {
        return true;
    }

    match (source_modified, destination_metadata.modified()) {
        (Some(source_modified), Ok(destination_modified)) => destination_modified < source_modified,
        _ => false,
    }
}

fn handle_sync_error(source_path: &Path, destination: &Path, err: io::Error) {
    if err.raw_os_error() == Some(32) {
        println!(
            "cargo:warning=sidecar sync skipped for {} because it is locked by a running process. Close the active SubGen app or stop lingering subgen-sidecar.exe processes to pick up {}.",
            destination.display(),
            source_path.display()
        );
        return;
    }

    panic!(
        "failed to sync sidecar from {} to {}: {}",
        source_path.display(),
        destination.display(),
        err
    );
}

fn format_go_build_error(err: &io::Error) -> String {
    if err.kind() == io::ErrorKind::NotFound {
        return "failed to invoke go build for the sidecar: `go` was not found on PATH. Install Go 1.26.1 or newer and ensure the active shell can run `go version` before starting `tauri dev`.".to_string();
    }

    format!("failed to invoke go build for the sidecar: {}", err)
}
