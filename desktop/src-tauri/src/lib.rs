use tauri::{Emitter, Manager};
#[cfg(desktop)]
use tauri::WindowEvent;
#[cfg(desktop)]
use std::sync::atomic::{AtomicBool, Ordering};
#[cfg(desktop)]
use std::time::Duration;
use tauri::RunEvent;
use tauri_plugin_deep_link::DeepLinkExt;

// Set once when the app begins shutting down so we don't loop on the second
// ExitRequested that fires from our own delayed app.exit(0). Also lets the
// tray Quit and ExitRequested paths share the same emit-then-exit logic.
#[cfg(desktop)]
static EXITING: AtomicBool = AtomicBool::new(false);

// begin_graceful_exit notifies the renderer ("parley:quitting") so it can
// flush a voice-leave beacon (and any other pre-exit cleanup), then exits
// after a short delay. Idempotent — repeated calls are no-ops.
#[cfg(desktop)]
fn begin_graceful_exit<R: tauri::Runtime>(app: &tauri::AppHandle<R>) {
    if EXITING.swap(true, Ordering::SeqCst) {
        return;
    }
    let _ = app.emit("parley:quitting", ());
    let h = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(Duration::from_millis(250));
        h.exit(0);
    });
}

mod ring_window;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let mut builder = tauri::Builder::default();

    #[cfg(desktop)]
    {
        builder = builder.plugin(tauri_plugin_single_instance::init(|app, argv, _cwd| {
            let _ = app.get_webview_window("main").and_then(|w| {
                let _ = w.show();
                let _ = w.set_focus();
                Some(w)
            });
            if let Some(url) = argv.iter().find(|a| a.starts_with("parley://")) {
                let _ = app.emit("deep-link", url.clone());
            }
        }));
    }

    #[cfg(desktop)]
    {
        builder = builder
            .plugin(tauri_plugin_updater::Builder::new().build())
            .plugin(tauri_plugin_process::init());
    }

    builder
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_os::init())
        .invoke_handler({
            #[cfg(not(any(target_os = "ios", target_os = "android")))]
            { tauri::generate_handler![ring_window::spawn_ring_window, ring_window::dismiss_ring_window] }
            #[cfg(any(target_os = "ios", target_os = "android"))]
            { tauri::generate_handler![] }
        })
        .setup(|app| {
            let handle = app.handle().clone();
            app.deep_link().on_open_url(move |event| {
                for url in event.urls() {
                    let _ = handle.emit("deep-link", url.to_string());
                }
            });

            #[cfg(desktop)]
            setup_tray(app)?;

            // Desktop close-to-tray: keep the process alive when the window is
            // closed so WebSocket + OS notifications keep working. We emit
            // parley:foreground=false just before hide() so the renderer's
            // notification hook flips its flag — DOM and tauri://blur signals
            // don't reliably fire on macOS hide. Mobile has no window-close
            // concept (iOS pauses the app via the lifecycle instead), so the
            // whole handler is desktop-only.
            #[cfg(desktop)]
            if let Some(window) = app.get_webview_window("main") {
                let hide_target = window.clone();
                window.on_window_event(move |event| {
                    if let WindowEvent::CloseRequested { api, .. } = event {
                        api.prevent_close();
                        let _ = hide_target.emit("parley:foreground", false);
                        let _ = hide_target.hide();
                    }
                });
            }

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|_app, _event| {
            // Cmd+Q (macOS), Alt+F4 (Windows), and OS shutdown all surface as
            // ExitRequested. Intercept the first one to give the renderer
            // ~250ms to fire a voice-leave beacon, then exit. Subsequent
            // ExitRequested events (from our own delayed app.exit(0)) pass
            // through because EXITING is already set.
            #[cfg(desktop)]
            if let RunEvent::ExitRequested { api, .. } = &_event {
                if !EXITING.load(Ordering::SeqCst) {
                    api.prevent_exit();
                    begin_graceful_exit(_app);
                }
            }

            // macOS: clicking the Dock icon after closing (hiding) the main
            // window fires applicationShouldHandleReopen. We re-show the
            // hidden window so the Dock click matches the tray-click path.
            // RunEvent::Reopen only exists on macOS, so the whole branch is
            // gated off — other platforms fall through.
            #[cfg(target_os = "macos")]
            if let RunEvent::Reopen { has_visible_windows, .. } = _event {
                if !has_visible_windows {
                    reveal_main_window(_app);
                }
            }
        });
}

#[cfg(desktop)]
fn setup_tray(app: &tauri::App) -> Result<(), Box<dyn std::error::Error>> {
    use tauri::image::Image;
    use tauri::menu::{MenuBuilder, MenuItemBuilder};
    use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};

    let show_item = MenuItemBuilder::with_id("show", "Show Parley").build(app)?;
    let quit_item = MenuItemBuilder::with_id("quit", "Quit Parley").build(app)?;
    let menu = MenuBuilder::new(app).items(&[&show_item, &quit_item]).build()?;

    // Monochrome P-only glyph (black on transparent). On macOS this is rendered
    // as an NSImage template so the system tints it to match the menu bar
    // (white on dark, dark on light). Windows/Linux get the same shape, which
    // reads fine against their tray backgrounds.
    let tray_icon = Image::from_bytes(include_bytes!("../icons/tray-template.png"))?;

    let _tray = TrayIconBuilder::with_id("parley-tray")
        .icon(tray_icon)
        .icon_as_template(cfg!(target_os = "macos"))
        .tooltip("Parley")
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "show" => reveal_main_window(app),
            "quit" => begin_graceful_exit(app),
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                reveal_main_window(tray.app_handle());
            }
        })
        .build(app)?;

    Ok(())
}

#[cfg(desktop)]
fn reveal_main_window<R: tauri::Runtime>(app: &tauri::AppHandle<R>) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.show();
        let _ = w.unminimize();
        let _ = w.set_focus();
        let _ = w.emit("parley:foreground", true);
    }
}
