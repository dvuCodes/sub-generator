use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use tauri::Emitter;
use tauri::Manager;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

struct SidecarState {
    child: Arc<Mutex<Option<CommandChild>>>,
}

#[tauri::command]
async fn spawn_sidecar(
    app: tauri::AppHandle,
    state: tauri::State<'_, SidecarState>,
) -> Result<(), String> {
    let mut child_lock = state.child.lock().map_err(|e| e.to_string())?;

    if child_lock.is_some() {
        return Ok(());
    }

    let sidecar = app
        .shell()
        .sidecar("subgen-sidecar")
        .map_err(|e| format!("Failed to create sidecar command: {}", e))?;

    let workdir = resolve_sidecar_workdir(&app)?;

    let (mut rx, child) = sidecar
        .current_dir(workdir)
        .spawn()
        .map_err(|e| format!("Failed to spawn sidecar: {}", e))?;

    *child_lock = Some(child);
    drop(child_lock);

    let app_handle = app.clone();
    let child_state = Arc::clone(&state.child);

    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line) => {
                    let line_str = String::from_utf8_lossy(&line);
                    let _ = app_handle.emit("sidecar-output", line_str.to_string());
                }
                CommandEvent::Stderr(line) => {
                    let line_str = String::from_utf8_lossy(&line);
                    let _ = app_handle.emit("sidecar-error", line_str.to_string());
                    eprintln!("[sidecar stderr] {}", line_str);
                }
                CommandEvent::Terminated(payload) => {
                    clear_child_state(&child_state);
                    let _ = app_handle.emit("sidecar-terminated", payload.code);
                    return;
                }
                CommandEvent::Error(err) => {
                    clear_child_state(&child_state);
                    let _ = app_handle.emit("sidecar-error", err.clone());
                    eprintln!("[sidecar error] {}", err);
                    return;
                }
                _ => {}
            }
        }

        clear_child_state(&child_state);
    });

    Ok(())
}

#[tauri::command]
async fn send_to_sidecar(
    message: String,
    state: tauri::State<'_, SidecarState>,
) -> Result<(), String> {
    let mut child_lock = state.child.lock().map_err(|e| e.to_string())?;

    if let Some(ref mut child) = *child_lock {
        let msg = if message.ends_with('\n') {
            message
        } else {
            format!("{}\n", message)
        };
        child
            .write(msg.as_bytes())
            .map_err(|e| format!("Failed to write to sidecar: {}", e))?;
        Ok(())
    } else {
        Err("Sidecar is not running".to_string())
    }
}

#[tauri::command]
async fn kill_sidecar(state: tauri::State<'_, SidecarState>) -> Result<(), String> {
    let mut child_lock = state.child.lock().map_err(|e| e.to_string())?;

    if let Some(child) = child_lock.take() {
        child
            .kill()
            .map_err(|e| format!("Failed to kill sidecar: {}", e))?;
    }
    Ok(())
}

fn clear_child_state<T>(child: &Arc<Mutex<Option<T>>>) {
    if let Ok(mut child_lock) = child.lock() {
        *child_lock = None;
    }
}

fn resolve_sidecar_workdir(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    if cfg!(debug_assertions) {
        return debug_sidecar_workdir();
    }

    app.path()
        .resource_dir()
        .map_err(|err| err.to_string())
        .or_else(|err| {
            std::env::current_dir().map_err(|cwd_err| {
                format!(
                    "Failed to resolve resource directory ({}) and current directory ({})",
                    err, cwd_err
                )
            })
        })
}

fn debug_sidecar_workdir() -> Result<PathBuf, String> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    manifest_dir
        .parent()
        .map(PathBuf::from)
        .ok_or_else(|| "Failed to resolve workspace root".to_string())
}

fn linux_webview_env_overrides(
    is_linux: bool,
    is_wsl: bool,
    has_compositing_override: bool,
    has_dmabuf_override: bool,
) -> Vec<(&'static str, &'static str)> {
    if !is_linux || !is_wsl {
        return Vec::new();
    }

    let mut overrides = Vec::with_capacity(2);
    if !has_compositing_override {
        overrides.push(("WEBKIT_DISABLE_COMPOSITING_MODE", "1"));
    }
    if !has_dmabuf_override {
        overrides.push(("WEBKIT_DISABLE_DMABUF_RENDERER", "1"));
    }

    overrides
}

fn configure_runtime_workarounds() {
    let overrides = linux_webview_env_overrides(
        cfg!(target_os = "linux"),
        std::env::var_os("WSL_DISTRO_NAME").is_some(),
        std::env::var_os("WEBKIT_DISABLE_COMPOSITING_MODE").is_some(),
        std::env::var_os("WEBKIT_DISABLE_DMABUF_RENDERER").is_some(),
    );

    for (key, value) in overrides {
        // Set the WebKitGTK rendering fallbacks before Tauri initializes the Linux webview.
        unsafe { std::env::set_var(key, value) };
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    configure_runtime_workarounds();

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(SidecarState {
            child: Arc::new(Mutex::new(None)),
        })
        .invoke_handler(tauri::generate_handler![
            spawn_sidecar,
            send_to_sidecar,
            kill_sidecar,
        ])
        .setup(|app| {
            if cfg!(debug_assertions) {
                app.handle().plugin(
                    tauri_plugin_log::Builder::default()
                        .level(log::LevelFilter::Info)
                        .build(),
                )?;
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(test)]
mod tests {
    use super::{clear_child_state, debug_sidecar_workdir, linux_webview_env_overrides};
    use std::path::PathBuf;
    use std::sync::{Arc, Mutex};

    #[test]
    fn clear_child_state_resets_tracked_process() {
        let child = Arc::new(Mutex::new(Some(42_u8)));

        clear_child_state(&child);

        assert!(child.lock().unwrap().is_none());
    }

    #[test]
    fn linux_webview_env_overrides_only_applies_in_wsl_linux() {
        let overrides = linux_webview_env_overrides(false, true, false, false);
        assert!(overrides.is_empty());

        let overrides = linux_webview_env_overrides(true, false, false, false);
        assert!(overrides.is_empty());
    }

    #[test]
    fn linux_webview_env_overrides_sets_missing_webkit_flags() {
        let overrides = linux_webview_env_overrides(true, true, false, false);

        assert_eq!(
            overrides,
            vec![
                ("WEBKIT_DISABLE_COMPOSITING_MODE", "1"),
                ("WEBKIT_DISABLE_DMABUF_RENDERER", "1"),
            ]
        );
    }

    #[test]
    fn linux_webview_env_overrides_preserves_existing_user_overrides() {
        let overrides = linux_webview_env_overrides(true, true, true, false);

        assert_eq!(overrides, vec![("WEBKIT_DISABLE_DMABUF_RENDERER", "1")]);
    }

    #[test]
    fn debug_sidecar_workdir_uses_workspace_root() {
        if !cfg!(debug_assertions) {
            return;
        }

        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let expected = manifest_dir.parent().unwrap().to_path_buf();

        assert_eq!(debug_sidecar_workdir().unwrap(), expected);
    }
}
