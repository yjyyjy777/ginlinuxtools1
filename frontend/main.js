// --- DOM 元素获取 ---
const ip = document.getElementById("ip");
const password = document.getElementById("password");
const user = document.getElementById("user");
const sshPort = document.getElementById("sshPort");

const localPort = document.getElementById("localPort");
const remotePort = document.getElementById("remotePort");

const startButton = document.getElementById("startButton");
const stopButton = document.getElementById("stopButton");
const browserButton = document.getElementById("browserButton");

const statusIndicator = document.getElementById("statusIndicator");
const statusMessage = document.getElementById("statusMessage");
const historySelect = document.getElementById("history");

const progressContainer = document.getElementById("progress-container");
const progressBar = document.getElementById("progress-bar");

// --- 图标资源 ---
const sunIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>`;
const moonIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>`;

// --- 状态控制 ---
let appIsRunning = false;

function showStatusMessage(msg, isDynamic = false) {
    statusMessage.textContent = msg;
    if (isDynamic) {
        statusMessage.classList.add('blinking-dots');
    } else {
        statusMessage.classList.remove('blinking-dots');
    }
}

function updateUIStatus(isRunning, local, remote) {
    appIsRunning = isRunning;
    statusMessage.classList.remove('blinking-dots');

    if (isRunning) {
        statusIndicator.classList.remove('error');
        statusIndicator.classList.add('running');
        statusMessage.textContent = `运行中：本地 ${local} → 远端 ${remote}`;
        startButton.disabled = true;
        stopButton.disabled = false;
        browserButton.disabled = false;
    } else {
        statusIndicator.classList.remove('running');
        statusMessage.textContent = "准备就绪";
        startButton.disabled = false;
        stopButton.disabled = true;
        browserButton.disabled = true;
    }
    // Hide progress bar when not running
    progressContainer.style.display = 'none';
}

// --- 主题切换逻辑 ---
const themeSwitcher = document.getElementById("theme-switcher");
const themeIcon = document.getElementById("theme-icon");

function setTheme(theme) {
    document.body.setAttribute('data-theme', theme);
    themeIcon.innerHTML = theme === 'dark' ? sunIcon : moonIcon;
    localStorage.setItem('theme', theme);
}

function toggleTheme() {
    const currentTheme = document.body.getAttribute('data-theme');
    const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
    setTheme(newTheme);
}

function loadTheme() {
    const savedTheme = localStorage.getItem('theme');
    const defaultTheme = 'light';
    setTheme(savedTheme || defaultTheme);
}
themeSwitcher.addEventListener('click', toggleTheme);

// --- Wails 后端事件监听 ---
if (window.runtime) {
    window.runtime.EventsOn("log:update", (msg) => {
        if (startButton.disabled && !appIsRunning) {
            showStatusMessage(msg, msg.includes('...'));
        }
        if (msg.includes("上传组件")) {
            progressContainer.style.display = 'block';
            progressBar.style.width = '0%';
        }
    });

    window.runtime.EventsOn("upload:progress", (progress) => {
        progressBar.style.width = `${progress}%`;
    });

    window.runtime.EventsOn("status:update", (data) => {
        updateUIStatus(data.isRunning, data.localPort, data.remotePort);
    });

    window.runtime.EventsOn("history:loaded", (list) => {
        historySelect.innerHTML = `<option value="">选择一个已保存的连接...</option>`;
        if (list && list.length > 0) {
            list.forEach(p => {
                const opt = document.createElement("option");
                opt.value = p.name;
                opt.textContent = p.name;
                opt.dataset.all = JSON.stringify(p);
                historySelect.appendChild(opt);
            });
            historySelect.selectedIndex = list.length;
            historySelect.dispatchEvent(new Event('change'));
        }
    });
}

// --- 历史记录选择事件 ---
historySelect.addEventListener("change", () => {
    const opt = historySelect.selectedOptions[0];
    if (!opt || !opt.dataset.all) return;

    try {
        const profile = JSON.parse(opt.dataset.all);
        ip.value = profile.host || "";
        sshPort.value = profile.port || "22";
        user.value = profile.user || "root";
        password.value = profile.password || "";
        localPort.value = profile.localPort || "";
        remotePort.value = profile.remotePort || "";
    } catch (e) {}
});

// --- 按钮点击事件 ---
startButton.addEventListener("click", async () => {
    if (!window.go) return;

    showStatusMessage("正在准备...", true);
    statusIndicator.classList.remove('running', 'error');
    startButton.disabled = true;
    stopButton.disabled = true;
    browserButton.disabled = true;
    progressContainer.style.display = 'none';
    progressBar.style.width = '0%';

    try {
        await window.go.main.App.StartDeploy(
            ip.value, sshPort.value, user.value, password.value, localPort.value, remotePort.value
        );
    } catch (err) {
        showStatusMessage("❌ 开始失败: " + err); // 使用 showStatusMessage
        statusIndicator.classList.add('error');
        updateUIStatus(false, "", "");
    }
});

stopButton.addEventListener("click", async () => {
    if (!window.go) return;
    showStatusMessage("正在停止...", true);
    startButton.disabled = true;
    stopButton.disabled = true;
    browserButton.disabled = true;

    try {
        await window.go.main.App.StopDeploy();
    } catch (err) {
        showStatusMessage("❌ 停止失败: " + err); // 使用 showStatusMessage
        statusIndicator.classList.add('error');
        updateUIStatus(false, "", "");
    }
});

browserButton.addEventListener("click", () => {
    if (!window.go) return;
    window.go.main.App.OpenBrowser();
});

// --- 页面初始化 ---
window.addEventListener('wails:ready', () => {
    loadTheme();
    statusMessage.textContent = "准备就绪";
});
