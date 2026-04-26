use serde::Deserialize;
use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindowBuilder};

#[derive(Debug, Deserialize)]
pub struct RingWindowArgs {
    pub ring_id: String,
    pub vc: String,
    pub caller_username: String,
    pub caller_display_name: String,
    pub caller_avatar_url: Option<String>,
    pub group_name: Option<String>,
}

fn ring_window_label(ring_id: &str) -> String {
    format!("ring-{}", ring_id)
}

// Secondary ring windows are desktop-only. Mobile builds use the in-app modal.
#[cfg(not(any(target_os = "ios", target_os = "android")))]
#[tauri::command]
pub async fn spawn_ring_window(app: AppHandle, args: RingWindowArgs) -> Result<(), String> {
    let label = ring_window_label(&args.ring_id);
    if app.get_webview_window(&label).is_some() {
        return Ok(()); // already open
    }
    // Build the URL with caller info as query string. ring.html is the entry.
    let avatar = args.caller_avatar_url.clone().unwrap_or_default();
    let group = args.group_name.clone().unwrap_or_default();
    let qs = format!(
        "ring_id={}&vc={}&caller_username={}&caller_display_name={}&caller_avatar_url={}&group_name={}",
        urlencoding::encode(&args.ring_id),
        urlencoding::encode(&args.vc),
        urlencoding::encode(&args.caller_username),
        urlencoding::encode(&args.caller_display_name),
        urlencoding::encode(&avatar),
        urlencoding::encode(&group),
    );
    let url = format!("ring.html?{}", qs);

    let monitor = app.primary_monitor().map_err(|e| e.to_string())?
        .ok_or("no primary monitor")?;
    let size = monitor.size();
    let scale = monitor.scale_factor();
    let win_w = 320.0;
    let win_h = 400.0;
    let x = (size.width as f64 / scale) - win_w - 24.0;
    let y = (size.height as f64 / scale) - win_h - 24.0;

    WebviewWindowBuilder::new(&app, &label, WebviewUrl::App(url.into()))
        .title("Incoming call")
        .inner_size(win_w, win_h)
        .resizable(false)
        .always_on_top(true)
        .skip_taskbar(true)
        .decorations(false)
        .transparent(true)
        .position(x, y)
        .focused(true)
        .build()
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[cfg(not(any(target_os = "ios", target_os = "android")))]
#[tauri::command]
pub async fn dismiss_ring_window(app: AppHandle, ring_id: String) -> Result<(), String> {
    let label = ring_window_label(&ring_id);
    if let Some(w) = app.get_webview_window(&label) {
        let _ = w.close();
    }
    Ok(())
}
