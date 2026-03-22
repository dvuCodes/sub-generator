use std::sync::{Arc, Mutex};
use tauri::Emitter;
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

    let (mut rx, child) = sidecar
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

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
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
    use super::clear_child_state;
    use std::sync::{Arc, Mutex};

    #[test]
    fn clear_child_state_resets_tracked_process() {
        let child = Arc::new(Mutex::new(Some(42_u8)));

        clear_child_state(&child);

        assert!(child.lock().unwrap().is_none());
    }
}
