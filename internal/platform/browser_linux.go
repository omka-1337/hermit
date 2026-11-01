//go:build linux && cgo && !gtk3

package platform

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <gtk/gtk.h>
#include <webkit/webkit.h>

extern void hermitBrowserNXM(uintptr_t handle, char *uri);
extern void hermitBrowserChanged(uintptr_t handle);
extern void hermitBrowserCrashed(uintptr_t handle, int reason);

typedef struct {
	WebKitWebView *view;
	uintptr_t handle;
} HermitBrowser;

// find_webview finds the app's own web view among a widget and its children.
static GtkWidget *find_webview(GtkWidget *w) {
	if (w == NULL)
		return NULL;
	if (WEBKIT_IS_WEB_VIEW(w))
		return w;
	for (GtkWidget *c = gtk_widget_get_first_child(w); c != NULL; c = gtk_widget_get_next_sibling(c)) {
		GtkWidget *found = find_webview(c);
		if (found != NULL)
			return found;
	}
	return NULL;
}

static int is_web(const char *uri) {
	return g_str_has_prefix(uri, "https://") || g_str_has_prefix(uri, "http://");
}

// on_decide_policy hands nxm:// links to Hermit instead of following them,
// and opens in the same view what a page would open in a window of its own.
static gboolean on_decide_policy(WebKitWebView *view, WebKitPolicyDecision *decision, WebKitPolicyDecisionType type, gpointer data) {
	HermitBrowser *b = data;
	if (type != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION && type != WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION)
		return FALSE;
	WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(decision));
	const char *uri = webkit_uri_request_get_uri(webkit_navigation_action_get_request(action));
	if (uri == NULL)
		return FALSE;
	if (g_ascii_strncasecmp(uri, "nxm:", 4) == 0) {
		webkit_policy_decision_ignore(decision);
		hermitBrowserNXM(b->handle, (char *)uri);
		return TRUE;
	}
	if (type == WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) {
		webkit_policy_decision_ignore(decision);
		if (is_web(uri))
			webkit_web_view_load_uri(view, uri);
		return TRUE;
	}
	if (!is_web(uri) && !g_str_has_prefix(uri, "about:") && !g_str_has_prefix(uri, "blob:") && !g_str_has_prefix(uri, "data:")) {
		webkit_policy_decision_ignore(decision);
		return TRUE;
	}
	return FALSE;
}

static void on_changed(GObject *object, GParamSpec *spec, gpointer data) {
	hermitBrowserChanged(((HermitBrowser *)data)->handle);
}

static void on_load_changed(WebKitWebView *view, WebKitLoadEvent event, gpointer data) {
	hermitBrowserChanged(((HermitBrowser *)data)->handle);
}

static void on_terminated(WebKitWebView *view, WebKitWebProcessTerminationReason reason, gpointer data) {
	hermitBrowserCrashed(((HermitBrowser *)data)->handle, (int)reason);
}

// hermit_browser_new puts a web view of its own over the app's in window,
// hidden until it is placed. The app's web view is moved into an overlay
// for it. The view shares nothing with the app's: its own web context,
// without the scheme the app's pages reach the app through, its own user
// content manager, without the app's message handlers, and its own network
// session, whose cookies are kept in dataDir.
static HermitBrowser *hermit_browser_new(void *window, const char *dataDir, const char *cacheDir, uintptr_t handle) {
	GtkWidget *app = find_webview(gtk_window_get_child(GTK_WINDOW(window)));
	if (app == NULL)
		return NULL;
	GtkWidget *parent = gtk_widget_get_parent(app);
	GtkWidget *overlay = parent;
	if (!GTK_IS_OVERLAY(parent)) {
		if (!GTK_IS_BOX(parent))
			return NULL;
		GtkWidget *before = gtk_widget_get_prev_sibling(app);
		g_object_ref(app);
		gtk_box_remove(GTK_BOX(parent), app);
		overlay = gtk_overlay_new();
		gtk_widget_set_hexpand(overlay, TRUE);
		gtk_widget_set_vexpand(overlay, TRUE);
		gtk_overlay_set_child(GTK_OVERLAY(overlay), app);
		g_object_unref(app);
		gtk_box_insert_child_after(GTK_BOX(parent), overlay, before);
	}

	WebKitNetworkSession *session = webkit_network_session_new(dataDir, cacheDir);
	char *cookies = g_build_filename(dataDir, "cookies.sqlite", NULL);
	webkit_cookie_manager_set_persistent_storage(webkit_network_session_get_cookie_manager(session), cookies, WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
	g_free(cookies);
	WebKitWebContext *context = webkit_web_context_new();
	WebKitUserContentManager *content = webkit_user_content_manager_new();
	WebKitWebView *view = WEBKIT_WEB_VIEW(g_object_new(WEBKIT_TYPE_WEB_VIEW,
		"network-session", session,
		"web-context", context,
		"user-content-manager", content,
		NULL));
	g_object_unref(session);
	g_object_unref(context);
	g_object_unref(content);

	// No sound or video: the pages are for downloading, and without the
	// audio output of GStreamer's good plugins, missing from many systems,
	// WebKit's process of a page that plays any crashes.
	WebKitSettings *settings = webkit_web_view_get_settings(view);
	webkit_settings_set_enable_media(settings, FALSE);
	webkit_settings_set_enable_webaudio(settings, FALSE);
	webkit_settings_set_enable_mediasource(settings, FALSE);
	webkit_settings_set_enable_encrypted_media(settings, FALSE);
	webkit_settings_set_enable_media_stream(settings, FALSE);
	webkit_settings_set_enable_webrtc(settings, FALSE);
	webkit_settings_set_media_playback_requires_user_gesture(settings, TRUE);

	gtk_widget_set_halign(GTK_WIDGET(view), GTK_ALIGN_START);
	gtk_widget_set_valign(GTK_WIDGET(view), GTK_ALIGN_START);
	gtk_widget_set_visible(GTK_WIDGET(view), FALSE);
	gtk_overlay_add_overlay(GTK_OVERLAY(overlay), GTK_WIDGET(view));

	HermitBrowser *b = g_new0(HermitBrowser, 1);
	b->view = view;
	b->handle = handle;
	g_signal_connect(view, "decide-policy", G_CALLBACK(on_decide_policy), b);
	g_signal_connect(view, "notify::uri", G_CALLBACK(on_changed), b);
	g_signal_connect(view, "notify::title", G_CALLBACK(on_changed), b);
	g_signal_connect(view, "notify::estimated-load-progress", G_CALLBACK(on_changed), b);
	g_signal_connect(view, "load-changed", G_CALLBACK(on_load_changed), b);
	g_signal_connect(view, "web-process-terminated", G_CALLBACK(on_terminated), b);
	return b;
}

static void hermit_browser_place(HermitBrowser *b, int x, int y, int w, int h) {
	GtkWidget *v = GTK_WIDGET(b->view);
	gtk_widget_set_margin_start(v, x);
	gtk_widget_set_margin_top(v, y);
	gtk_widget_set_size_request(v, w, h);
	gtk_widget_set_visible(v, TRUE);
}

static void hermit_browser_hide(HermitBrowser *b) { gtk_widget_set_visible(GTK_WIDGET(b->view), FALSE); }
static void hermit_browser_load(HermitBrowser *b, const char *uri) { webkit_web_view_load_uri(b->view, uri); }
static void hermit_browser_back(HermitBrowser *b) { webkit_web_view_go_back(b->view); }
static void hermit_browser_reload(HermitBrowser *b) { webkit_web_view_reload(b->view); }
static const char *hermit_browser_uri(HermitBrowser *b) { return webkit_web_view_get_uri(b->view); }
static const char *hermit_browser_title(HermitBrowser *b) { return webkit_web_view_get_title(b->view); }
static int hermit_browser_can_go_back(HermitBrowser *b) { return webkit_web_view_can_go_back(b->view); }
static int hermit_browser_loading(HermitBrowser *b) { return webkit_web_view_is_loading(b->view); }
static double hermit_browser_progress(HermitBrowser *b) { return webkit_web_view_get_estimated_load_progress(b->view); }
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"unsafe"
)

// Browser is a web view of its own inside the app's window, over the app's
// page, for websites the app sends the user to. It shares nothing with the
// app's pages: no way to reach the app, no cookies. Its methods must run on
// the GTK thread, and its callbacks run there.
type Browser struct {
	c *C.HermitBrowser
	h cgo.Handle
	// OnNXM gets the nxm:// links the page follows, which the browser does
	// not follow itself.
	OnNXM func(link string)
	// OnChange runs when the page, its title or how far it loaded change.
	OnChange func()
	// OnCrash runs when the process showing the page is gone, saying why;
	// Reload starts a new one.
	OnCrash func(reason string)
}

// BrowserState is what the browser shows.
type BrowserState struct {
	URI       string  `json:"uri"`
	Title     string  `json:"title"`
	CanGoBack bool    `json:"canGoBack"`
	Loading   bool    `json:"loading"`
	Progress  float64 `json:"progress"`
}

// NewBrowser puts a browser into nativeWindow, the app's GTK window,
// keeping its cookies and storage in dataDir and its cache in cacheDir.
func NewBrowser(nativeWindow unsafe.Pointer, dataDir, cacheDir string) (*Browser, error) {
	b := &Browser{}
	b.h = cgo.NewHandle(b)
	d, c := C.CString(dataDir), C.CString(cacheDir)
	defer C.free(unsafe.Pointer(d))
	defer C.free(unsafe.Pointer(c))
	b.c = C.hermit_browser_new(nativeWindow, d, c, C.uintptr_t(b.h))
	if b.c == nil {
		b.h.Delete()
		return nil, errors.New("the window has no web view to put a browser over")
	}
	return b, nil
}

// Place shows the browser at x, y of the window's page, w by h large.
func (b *Browser) Place(x, y, w, h int) {
	C.hermit_browser_place(b.c, C.int(x), C.int(y), C.int(max(1, w)), C.int(max(1, h)))
}

// Hide takes the browser off the page; it keeps what it shows.
func (b *Browser) Hide() { C.hermit_browser_hide(b.c) }

// Load opens a page.
func (b *Browser) Load(uri string) {
	u := C.CString(uri)
	defer C.free(unsafe.Pointer(u))
	C.hermit_browser_load(b.c, u)
}

func (b *Browser) Back()   { C.hermit_browser_back(b.c) }
func (b *Browser) Reload() { C.hermit_browser_reload(b.c) }

// State is what the browser shows now.
func (b *Browser) State() BrowserState {
	return BrowserState{
		URI:       C.GoString(C.hermit_browser_uri(b.c)),
		Title:     C.GoString(C.hermit_browser_title(b.c)),
		CanGoBack: C.hermit_browser_can_go_back(b.c) != 0,
		Loading:   C.hermit_browser_loading(b.c) != 0,
		Progress:  float64(C.hermit_browser_progress(b.c)),
	}
}

//export hermitBrowserNXM
func hermitBrowserNXM(handle C.uintptr_t, uri *C.char) {
	b := cgo.Handle(handle).Value().(*Browser)
	if b.OnNXM != nil {
		b.OnNXM(C.GoString(uri))
	}
}

//export hermitBrowserCrashed
func hermitBrowserCrashed(handle C.uintptr_t, reason C.int) {
	b := cgo.Handle(handle).Value().(*Browser)
	if b.OnCrash == nil {
		return
	}
	switch reason {
	case C.WEBKIT_WEB_PROCESS_EXCEEDED_MEMORY_LIMIT:
		b.OnCrash("the page took more memory than it may")
	case C.WEBKIT_WEB_PROCESS_TERMINATED_BY_API:
		b.OnCrash("the page was stopped")
	default:
		b.OnCrash("the page crashed")
	}
}

//export hermitBrowserChanged
func hermitBrowserChanged(handle C.uintptr_t) {
	b := cgo.Handle(handle).Value().(*Browser)
	if b.OnChange != nil {
		b.OnChange()
	}
}
