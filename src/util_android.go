//go:build android

package main

/*
#cgo CFLAGS: -DSDL_MAIN_HANDLED
#cgo LDFLAGS: -lEGL -landroid -llog
#include <EGL/egl.h>
#include <jni.h>
#include <android/log.h>
#include <stdlib.h>
#include <unistd.h>
#include "SDL.h"

static const char* GetStringUTFChars_Wrapper(JNIEnv* env, jstring str) {
    if (!str) return NULL;
    return (*env)->GetStringUTFChars(env, str, NULL);
}

static void ReleaseStringUTFChars_Wrapper(JNIEnv* env, jstring str, const char* chars) {
    if (str && chars) {
        (*env)->ReleaseStringUTFChars(env, str, chars);
    }
}

// THIS IS THE KEY: A C constructor.
// This runs when the .so is loaded, BEFORE Go starts itself up.
// 替换 util_android.go 中的 prepare_go_runtime()

__attribute__((constructor))
static void prepare_go_runtime() {
    setenv("GODEBUG", "asyncpreemptoff=1,sigaltstack=0,cgocheck=0,scavenge=off,installgoroot=0,hardstacklimit=0", 1);
    
    // 新增：写入文件日志（无需Root查看）
    const char* base_path = "/sdcard/Android/data/org.ikemen_engine.ikemen_go/files/";
    
    // 确保目录存在
    mkdir(base_path, 0755);
    
    char log_path[256];
    snprintf(log_path, sizeof(log_path), "%s/ikemen_boot.log", base_path);
    
    FILE* f = fopen(log_path, "w");
    if (f) {
        time_t now = time(NULL);
        fprintf(f, "=== IKEMEN GO Boot Log ===\n");
        fprintf(f, "Time: %s", ctime(&now));
        
        char prop[PROP_VALUE_MAX];
        
        __system_property_get("ro.build.version.release", prop);
        fprintf(f, "Android: %s\n", prop);
        
        __system_property_get("ro.product.model", prop);
        fprintf(f, "Model: %s\n", prop);
        
        __system_property_get("ro.product.manufacturer", prop);
        fprintf(f, "Manufacturer: %s\n", prop);
        
        fprintf(f, "GODEBUG: %s\n", getenv("GODEBUG") ? getenv("GODEBUG") : "NOT SET");
        fprintf(f, "==========================\n");
        fclose(f);
    }
}

*/
import "C"
import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	findfont "github.com/flopp/go-findfont"
)

var (
	extractionDone = make(chan bool, 1)
	baseDir        string
)

func init() {
}

// Log writer implementation
// On Android, writing to Stderr usually redirects to Logcat
func NewLogWriter() io.Writer {
	return os.Stderr
}

// TTF font loading
func LoadFntTtf(f *Fnt, fontfile string, filename string, height int32) {
	// 1. Path resolution
	// Android paths are tricky.
	// Attempt to find the file using the engine's SearchFile utility
	fileDir := SearchFile(filename, []string{fontfile, sys.motif.Def, "", "data/", "font/"})

	// 2. Android Path Correction
	// If the path isn't absolute and doesn't exist, anchor it to the SDL storage path
	if !filepath.IsAbs(fileDir) && FileExist(fileDir) == "" {
		fullPath := filepath.Join(getAndroidFilesDir(), fileDir)

		if FileExist(fullPath) != "" {
			fileDir = fullPath
		}
	}

	// Fallback to findfont if still not found
	if FileExist(fileDir) == "" {
		if found, err := findfont.Find(fileDir); err == nil {
			fileDir = found
		} else {
			Logcat(fmt.Sprintf("Font search failed for %s, trying direct path...", filename))
		}
	}

	// 2. Set dimensions
	if height == -1 {
		height = int32(f.Size[1])
	} else {
		f.Size[1] = uint16(height)
	}

	// 3. Load the TTF
	ttf, err := gfxFont.LoadFont(fileDir, height, int(sys.gameWidth), int(sys.gameHeight))
	if err != nil {
		// Instead of a pure panic, log exactly where it looked
		Logcat(fmt.Sprintf("ERROR: Failed to load TTF from %s", fileDir))
		panic(fmt.Errorf("failed to load ttf font %v: %w", fileDir, err))
	}

	f.ttf = ttf.(Font)

	// 4. Create dummy palettes
	f.palettes = make([][256]uint32, 1)
	for i := 0; i < 256; i++ {
		f.palettes[0][i] = 0
	}
}

// Message box implementation
// Android doesn't have a simple "dialog" package like desktop.
func ShowInfoDialog(message, title string) {
	Logcat(fmt.Sprintf("INFO [%s]: %s", title, message))
}

func ShowErrorDialog(message string) {
	Logcat(fmt.Sprintf("CRITICAL ERROR: %s", message))
}

//export SDL_main
func SDL_main(argc C.int, argv **C.char) C.int {
	runtime.LockOSThread()

	// Wait for JNI to tell us the path is ready
	<-extractionDone

	Logcat("SDL_main: Path ready, jumping to realMain")

	// Set the baseDir before starting
	sys.baseDir = baseDir

	// Call realMain NOW that we're on the main thread
	realMain()

	return 0
}

func eglGetProcAddress(name string) unsafe.Pointer {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return unsafe.Pointer(C.eglGetProcAddress(cname))
}

func selectRenderer(cfgVal string) (Renderer, FontRenderer) {
	return &Renderer_GLES32{}, &FontRenderer_GLES32{}
}

func getAndroidFilesDir() string {
	path := C.SDL_AndroidGetExternalStoragePath()
	return C.GoString(path)
}

//export Java_org_libsdl_app_SDLActivity_nativeOnSDLReady
func Java_org_libsdl_app_SDLActivity_nativeOnSDLReady(env *C.JNIEnv, clazz C.jclass, jPath C.jstring) {
	cPath := C.GetStringUTFChars_Wrapper(env, jPath)
	if cPath != nil {
		baseDir = C.GoString(cPath)
		C.ReleaseStringUTFChars_Wrapper(env, jPath, cPath)
	}

	// Signal the Go thread to wake up
	select {
	case extractionDone <- true:
		Logcat("JNI: Signal sent to Go")
	default:
		Logcat("JNI: Signal already sent")
	}
}

func Logcat(s string) {
	cs := C.CString(s)
	C.__android_log_write(C.ANDROID_LOG_INFO, C.CString("ikemen"), cs)
	C.free(unsafe.Pointer(cs))
}
