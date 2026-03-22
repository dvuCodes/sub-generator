use std::sync::Mutex;
use tauri::Emitter;
use tauri_plugin_shell::ShellExt;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};

struct SidecarState {
    child: Mutex<Option<CommandChild>>,
}

#[tauri::command]
async fn spawn_sidecar(app: tauri::AppHandle, state: tauri::State<'_, SidecarState>) -> Result<(), String> {
    let mut child_lock = state.child.lock().map_err(|e| e.to_string())?;

    // Already running
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

    // Spawn a task to listen for sidecar output and emit events to the frontend
    let app_handle = app.clone();
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
                    let _ = app_handle.emit("sidecar-terminated", payload.code);
                    break;
                }
                CommandEvent::Error(err) => {
                    let _ = app_handle.emit("sidecar-error", err.clone());
                    eprintln!("[sidecar error] {}", err);
                }
                _ => {}
            }
        }
    });

    Ok(())
}

#[tauri::command]
async fn send_to_sidecar(message: String, state: tauri::State<'_, SidecarState>) -> Result<(), String> {
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

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(SidecarState {
            child: Mutex::new(None),
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
