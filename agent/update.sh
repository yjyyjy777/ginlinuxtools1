#!/bin/bash
# update.sh - UEM 全量与分项更新脚本

set -e

SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")
CURRENT="/opt/emm/current"

BASIC_SERVICES=("mysqld" "redis" "rabbitmq-server" "minio" "nginx")
UEM_SERVICES=("EMMBackend" "MCMServer" "Platform_java" "AppServer" "tomcat" "scep-go")

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*"; }

declare -A MOD_MAP=(
    [AppServer]="AppServer"
    [emm_backend]="emm-server-main"
    [licenseServer]="license"
    [mcmServer]="mcmserver"
    [Platform_java]="platform"
)

# 1. 参数校验
if [[ $# -eq 0 ]]; then
    echo "========================================================"
    echo "用法: $0 <组件名> [参数]"
    echo "全量更新: $0 uem <keep-data | reset-data>"
    echo "         (keep-data: 保留数据并提示手动执行增量SQL; reset-data: 初始化数据库)"
    echo "分项更新: $0 webui [local]"
    echo "         $0 backend"
    echo "         $0 tomcat [local]"
    echo "         $0 <模块名> [local] (如 AppServer, mcmServer 等)"
    echo "========================================================"
    exit 1
fi

TARGET=$1
SUB_PARAM=${2:-}
IS_LOCAL=0
if [[ "$SUB_PARAM" == "local" ]]; then
    IS_LOCAL=1
fi

if [[ "$TARGET" == "uem" ]]; then
    if [[ "$SUB_PARAM" != "keep-data" && "$SUB_PARAM" != "reset-data" ]]; then
        echo "错误: 全量更新必须指定数据保留策略 (keep-data 或 reset-data)"
        echo "例如: $0 uem keep-data"
        exit 1
    fi
    DB_MODE="$SUB_PARAM"
fi

# 确保以 root 运行
if [[ $EUID -ne 0 ]]; then
    log "❌ Please run as root."
    exit 1
fi

# 停止服务逻辑
stop_services() {
    log "正在停止服务..."
    for svc in "${BASIC_SERVICES[@]}"; do
        if systemctl is-active --quiet "$svc"; then systemctl stop "$svc" 2>/dev/null || true; fi
    done
    for svc in "${UEM_SERVICES[@]}"; do
        if systemctl is-active --quiet "$svc"; then systemctl stop "$svc" 2>/dev/null || true; fi
    done
}

start_services() {
    log "正在启动服务..."
    for svc in "${BASIC_SERVICES[@]}"; do
        log "启动 $svc..."
        systemctl start "$svc" 2>/dev/null || log "警告: $svc 启动失败"
        sleep 2
    done
    for svc in "${UEM_SERVICES[@]}"; do
        log "启动 $svc..."
        systemctl start "$svc" 2>/dev/null || log "警告: $svc 启动失败"
    done
}

get_uem_pkg() {
    local uem_pkg=$(find "$BASEDIR" -maxdepth 1 -type f -name 'UEM-*.tar.gz' | sort | tail -n1)
    if [[ -z "$uem_pkg" || ! -f "$uem_pkg" ]]; then
        log "未找到 UEM 更新包 (UEM-*.tar.gz)，请检查目录"
        exit 1
    fi
    echo "$uem_pkg"
}

# 全量更新逻辑
do_update_uem() {
    local uem_pkg=$(get_uem_pkg)
    log "开始全量 UEM 更新: $uem_pkg"
    
    stop_services
    
    # 解压 UEM
    local TMPDIR=$(mktemp -d /tmp/uem_tmp_XXXX)
    tar -zxvf "$uem_pkg" -C "$TMPDIR" >/dev/null
    local TMP_CONTENT=$(find "$TMPDIR" -mindepth 1 -maxdepth 1 -type d | head -n1)
    
    local NEW_RELEASE_DIR="/opt/emm/release_$(date +%Y%m%d%H%M%S)"
    log "准备新版本目录: $NEW_RELEASE_DIR"
    mkdir -p "$NEW_RELEASE_DIR"
    
    if [[ -d "$CURRENT" ]]; then
        log "正在迁移基础环境..."
        for file in "$CURRENT"/*; do
            [[ ! -e "$file" ]] && continue
            name=$(basename "$file")
            case "$name" in
                "source") log "跳过旧 source 目录" ;;
                "config") log "跳过旧 config 目录" ;;
                "tomcat")
                    cp -rf "$file" "$NEW_RELEASE_DIR/"
                    rm -rf "$NEW_RELEASE_DIR/tomcat/webapps/"*
                    ;;
                *)
                    cp -rf "$file" "$NEW_RELEASE_DIR/"
                    ;;
            esac
        done
    fi
    
    mkdir -p "$NEW_RELEASE_DIR/source"
    
    # 更新模块
    for MOD in "${!MOD_MAP[@]}"; do
        local TAR_PREFIX=${MOD_MAP[$MOD]}
        local MOD_PKG=$(find "$TMP_CONTENT" -maxdepth 1 -type f -name "${TAR_PREFIX}-*" | head -n1)
        mkdir -p "$NEW_RELEASE_DIR/source/$MOD"
        if [[ -n "$MOD_PKG" && -f "$MOD_PKG" ]]; then
            tar -zxvf "$MOD_PKG" -C "$NEW_RELEASE_DIR/source/$MOD" --strip-components=1 >/dev/null
        fi
    done
    
    # 更新 WebUI
    local WEBUI_TAR=$(find "$TMP_CONTENT" -maxdepth 1 -type f -name 'WebUI*.tar.gz' | head -n1)
    if [[ -n "$WEBUI_TAR" && -f "$WEBUI_TAR" ]]; then
        mkdir -p "$NEW_RELEASE_DIR/source/WebUI"
        tar -zxvf "$WEBUI_TAR" -C "$NEW_RELEASE_DIR/source/WebUI" --strip-components=1 >/dev/null
    fi
    
    # 更新 WAR
    mkdir -p "$NEW_RELEASE_DIR/tomcat/webapps"
    while IFS=  read -r -d $'\0'; do
        cp -f "$REPLY" "$NEW_RELEASE_DIR/tomcat/webapps/"
    done < <(find "$TMP_CONTENT" -maxdepth 1 -name "*.war" -print0)
    
    # 处理配置合并
    local CONFIG_TAR=$(find "$TMP_CONTENT" -maxdepth 1 -type f -name 'config.tar.gz' | head -n1)
    if [[ -n "$CONFIG_TAR" && -f "$CONFIG_TAR" ]]; then
        mkdir -p "$NEW_RELEASE_DIR/config"
        # 复制旧配置文件
        if [[ -d "$CURRENT/config" ]]; then
            cp -rf "$CURRENT/config/"* "$NEW_RELEASE_DIR/config/" 2>/dev/null || true
        fi
        
        local TMP_CONFIG=$(mktemp -d /tmp/config_tmp_XXXX)
        tar -zxvf "$CONFIG_TAR" -C "$TMP_CONFIG" >/dev/null
        local NEW_CONF_DIR="$TMP_CONFIG"
        if [[ -d "$TMP_CONFIG/config" ]]; then NEW_CONF_DIR="$TMP_CONFIG/config"; fi
        
        # 智能合并 global.properties
        local OLD_GLOBAL="$NEW_RELEASE_DIR/config/global.properties"
        local NEW_GLOBAL="$NEW_CONF_DIR/global.properties"
        if [[ -f "$OLD_GLOBAL" && -f "$NEW_GLOBAL" ]]; then
            log "正在智能合并 global.properties..."
            
            # 更新 mdm_version 为新包中的版本
            local new_version=$(grep -E '^[[:space:]]*mdm_version[[:space:]]*=' "$NEW_GLOBAL" | cut -d'=' -f2- | tr -d '\r' | xargs)
            if [[ -n "$new_version" ]]; then
                if grep -q "^[[:space:]]*mdm_version[[:space:]]*=" "$OLD_GLOBAL"; then
                    sed -i "s|^[[:space:]]*mdm_version[[:space:]]*=.*|mdm_version=$new_version|" "$OLD_GLOBAL"
                else
                    echo "mdm_version=$new_version" >> "$OLD_GLOBAL"
                fi
                echo -e "\033[32m已更新版本号 mdm_version: $new_version\033[0m"
            fi

            # 读取新配置的所有有效 key (忽略注释和空行)
            while IFS='=' read -r key val; do
                # 剔除首尾空格
                key=$(echo "$key" | xargs)
                [[ -z "$key" || "$key" =~ ^#.* ]] && continue
                [[ "$key" == "mdm_version" ]] && continue
                
                # 如果旧配置中没有这个 key，则追加
                if ! grep -q "^[[:space:]]*$key[[:space:]]*=" "$OLD_GLOBAL"; then
                    echo "$key=$val" >> "$OLD_GLOBAL"
                    echo -e "\033[32m发现新配置项并已追加: $key=$val\033[0m"
                fi
            done < "$NEW_GLOBAL"
        fi
        
        # 将新 config 中的其他文件（不覆盖已存在的旧文件）拷贝过来
        cp -rn "$NEW_CONF_DIR/"* "$NEW_RELEASE_DIR/config/" 2>/dev/null || true
        rm -rf "$TMP_CONFIG"
    fi
    
    # 恢复 License
    local OLD_LIC_FILES="$CURRENT/source/licenseServer/files"
    local NEW_LIC_DIR="$NEW_RELEASE_DIR/source/licenseServer"
    if [[ -d "$OLD_LIC_FILES" && -d "$NEW_LIC_DIR" ]]; then
        rm -rf "$NEW_LIC_DIR/files"
        cp -rf "$OLD_LIC_FILES" "$NEW_LIC_DIR/"
    fi
    local OLD_FEATURE_DIR="$CURRENT/source/licenseServer/feature"
    local NEW_FEATURE_DIR="$NEW_RELEASE_DIR/source/licenseServer/feature"
    if [[ -d "$OLD_FEATURE_DIR" ]]; then
        mkdir -p "$NEW_FEATURE_DIR"
        cp -rf "$OLD_FEATURE_DIR"/* "$NEW_FEATURE_DIR/" 2>/dev/null || true
    fi
    
    # 数据库脚本处理
    local DBSCRIPT_TAR=$(find "$TMP_CONTENT" -maxdepth 1 -type f -name 'dbScript.tar.gz' | head -n1)
    local HAS_INCREMENT_DB=0
    local HAS_RESET_DB=0
    if [[ -n "$DBSCRIPT_TAR" && -f "$DBSCRIPT_TAR" ]]; then
        if [[ "$DB_MODE" == "reset-data" ]]; then
            log "数据策略: reset-data，提取全量初始化 SQL 脚本..."
            mkdir -p "$BASEDIR/dbscript"
            tar -zxvf "$DBSCRIPT_TAR" -C "$BASEDIR/dbscript/" --strip-components=1 >/dev/null
            HAS_RESET_DB=1
        elif [[ "$DB_MODE" == "keep-data" ]]; then
            log "数据策略: keep-data，提取数据库增量脚本..."
            mkdir -p "$BASEDIR/dbscript"
            tar -zxvf "$DBSCRIPT_TAR" -C "$BASEDIR/dbscript/" --strip-components=1 >/dev/null
            HAS_INCREMENT_DB=1
        fi
    fi
    
    rm -rf "$TMPDIR"
    
    # 仅变更 current 软链接指向，保留旧版本物理目录不受影响
    log "更新 current 软链接指向: $NEW_RELEASE_DIR"
    ln -snf "$NEW_RELEASE_DIR" "$CURRENT"
    chown -R root:root "$NEW_RELEASE_DIR"
    chmod -R 777 "$NEW_RELEASE_DIR"
    chown -R tomcat:tomcat "$NEW_RELEASE_DIR/tomcat"
    
    start_services
    log "=== 全量 UEM 更新完成 ==="
    
    # 解析数据库连接信息用于生成命令范例
    local DB_HOST="127.0.0.1"
    local DB_PORT="3306"
    local DB_USER="mdm"
    local DB_PASS=""
    local DB_NAME="mdm"

    local MULTI_HOST="127.0.0.1"
    local MULTI_PORT="3306"
    local MULTI_USER="mdm"
    local MULTI_PASS=""
    local MULTI_NAME="multitenant"

    local GLOBAL_PROP="$CURRENT/config/global.properties"
    
    if [[ -f "$GLOBAL_PROP" ]]; then
        local parsed_user=$(grep -E '^[[:space:]]*jdbc\.username[[:space:]]*=' "$GLOBAL_PROP" | cut -d'=' -f2- | tr -d '\r' | xargs)
        local parsed_pass=$(grep -E '^[[:space:]]*jdbc\.password[[:space:]]*=' "$GLOBAL_PROP" | cut -d'=' -f2- | tr -d '\r' | xargs)
        local parsed_url=$(grep -E '^[[:space:]]*jdbc\.url[[:space:]]*=' "$GLOBAL_PROP" | cut -d'=' -f2- | tr -d '\r' | xargs)
        
        if [[ -n "$parsed_user" ]]; then DB_USER="$parsed_user"; fi
        if [[ -n "$parsed_pass" ]]; then DB_PASS="$parsed_pass"; fi
        if [[ -n "$parsed_url" ]]; then
            if [[ "$parsed_url" =~ jdbc:mysql://([^:/]+)(:([0-9]+))?/([^?]+) ]]; then
                DB_HOST="${BASH_REMATCH[1]}"
                if [[ -n "${BASH_REMATCH[3]}" ]]; then DB_PORT="${BASH_REMATCH[3]}"; else DB_PORT="3306"; fi
                DB_NAME="${BASH_REMATCH[4]}"
            fi
        fi

        local parsed_muser=$(grep -E '^[[:space:]]*jdbc\.multitenant\.username[[:space:]]*=' "$GLOBAL_PROP" | cut -d'=' -f2- | tr -d '\r' | xargs)
        local parsed_mpass=$(grep -E '^[[:space:]]*jdbc\.multitenant\.password[[:space:]]*=' "$GLOBAL_PROP" | cut -d'=' -f2- | tr -d '\r' | xargs)
        local parsed_murl=$(grep -E '^[[:space:]]*jdbc\.multitenant\.url[[:space:]]*=' "$GLOBAL_PROP" | cut -d'=' -f2- | tr -d '\r' | xargs)

        if [[ -n "$parsed_muser" ]]; then MULTI_USER="$parsed_muser"; else MULTI_USER="$DB_USER"; fi
        if [[ -n "$parsed_mpass" ]]; then MULTI_PASS="$parsed_mpass"; else MULTI_PASS="$DB_PASS"; fi
        if [[ -n "$parsed_murl" ]]; then
            if [[ "$parsed_murl" =~ jdbc:mysql://([^:/]+)(:([0-9]+))?/([^?]+) ]]; then
                MULTI_HOST="${BASH_REMATCH[1]}"
                if [[ -n "${BASH_REMATCH[3]}" ]]; then MULTI_PORT="${BASH_REMATCH[3]}"; else MULTI_PORT="3306"; fi
                MULTI_NAME="${BASH_REMATCH[4]}"
            fi
        else
            MULTI_HOST="$DB_HOST"
            MULTI_PORT="$DB_PORT"
        fi
    fi

    local DB_PASS_ARG=""
    if [[ -n "$DB_PASS" ]]; then DB_PASS_ARG="-p${DB_PASS}"; else DB_PASS_ARG="-p"; fi
    local CMD_MDM="mysql -u${DB_USER} ${DB_PASS_ARG} -h${DB_HOST} -P${DB_PORT} ${DB_NAME}"

    local MULTI_PASS_ARG=""
    if [[ -n "$MULTI_PASS" ]]; then MULTI_PASS_ARG="-p${MULTI_PASS}"; else MULTI_PASS_ARG="-p"; fi
    local CMD_MULTI="mysql -u${MULTI_USER} ${MULTI_PASS_ARG} -h${MULTI_HOST} -P${MULTI_PORT} ${MULTI_NAME}"

    if [[ "$HAS_INCREMENT_DB" -eq 1 ]]; then
        local mysql_uem_dir=$(find "$BASEDIR/dbscript" -maxdepth 1 -type d -name "Mysql-UEM-*" 2>/dev/null | head -n1)
        local default_upgrade_dir="$BASEDIR/dbscript/Mysql-UEM-6.2.0/upgrade"
        if [[ -n "$mysql_uem_dir" ]]; then
            default_upgrade_dir="$mysql_uem_dir/upgrade"
        fi

        local mdm_upgrade_path="$default_upgrade_dir/mdm-upgrade.sql"
        local found_mdm_up=$(find "$BASEDIR/dbscript" -type f \( -iname "*mdm*upgrade*.sql" -o -iname "*upgrade*mdm*.sql" -o -iname "*update*mdm*.sql" \) 2>/dev/null | head -n1)
        if [[ -n "$found_mdm_up" ]]; then
            mdm_upgrade_path="$found_mdm_up"
        fi

        local multi_upgrade_path="$default_upgrade_dir/multitenant-upgrade.sql"
        local found_multi_up=$(find "$BASEDIR/dbscript" -type f \( -iname "*multitenant*upgrade*.sql" -o -iname "*upgrade*multitenant*.sql" -o -iname "*update*multitenant*.sql" \) 2>/dev/null | head -n1)
        if [[ -n "$found_multi_up" ]]; then
            multi_upgrade_path="$found_multi_up"
        fi
        
        echo -e "\n\033[31m===================================================================\033[0m"
        echo -e "\033[31m[注意] 全量升级完成，但数据库保留了原有数据 (keep-data)。\033[0m"
        echo -e "\033[31m请务必前往目录 $BASEDIR/dbscript，\033[0m"
        echo -e "\033[31m手动在数据库中执行增量升级的 SQL 脚本！\033[0m"
        echo -e "\033[31m命令范例:\033[0m"
        echo -e "\033[31m  $CMD_MDM < $mdm_upgrade_path\033[0m"
        echo -e "\033[31m  $CMD_MULTI < $multi_upgrade_path\033[0m"
        echo -e "\033[31m===================================================================\033[0m\n"
    elif [[ "$HAS_RESET_DB" -eq 1 ]]; then
        local mysql_uem_dir=$(find "$BASEDIR/dbscript" -maxdepth 1 -type d -name "Mysql-UEM-*" 2>/dev/null | head -n1)
        local default_sql_dir="$BASEDIR/dbscript/Mysql-UEM-6.2.0"
        if [[ -n "$mysql_uem_dir" ]]; then
            default_sql_dir="$mysql_uem_dir"
        fi

        local mdm_sql_path="$default_sql_dir/mdm.sql"
        local multi_sql_path="$default_sql_dir/multitenant.sql"
        local found_mdm=$(find "$BASEDIR/dbscript" -name "mdm.sql" 2>/dev/null | head -n1)
        if [[ -n "$found_mdm" ]]; then mdm_sql_path="$found_mdm"; fi
        local found_multi=$(find "$BASEDIR/dbscript" -name "multitenant.sql" 2>/dev/null | head -n1)
        if [[ -n "$found_multi" ]]; then multi_sql_path="$found_multi"; fi
        
        echo -e "\n\033[31m===================================================================\033[0m"
        echo -e "\033[31m[注意] 全量升级完成，数据策略为初始化 (reset-data)。\033[0m"
        echo -e "\033[31m初始化 SQL 脚本已提取至 $BASEDIR/dbscript 目录中。\033[0m"
        echo -e "\033[31m请务必手动进入数据库并执行初始化脚本！\033[0m"
        echo -e "\033[31m命令范例:\033[0m"
        echo -e "\033[31m  $CMD_MDM < $mdm_sql_path\033[0m"
        echo -e "\033[31m  $CMD_MULTI < $multi_sql_path\033[0m"
        echo -e "\033[31m===================================================================\033[0m\n"
    fi
}

do_update_webui() {
    log "开始更新 WebUI..."
    local PKG=""
    local TMPDIR=""
    if [[ $IS_LOCAL -eq 1 ]]; then
        PKG=$(find "$BASEDIR" -maxdepth 1 -type f \( -name 'WebUi.zip' -o -name 'WebUI*.zip' -o -name 'WebUI*.tar.gz' \) | head -n1)
        if [[ -z "$PKG" ]]; then log "未找到本地 WebUi 包"; exit 1; fi
        log "使用本地包: $PKG"
        rm -rf "$CURRENT/source/WebUI"
        mkdir -p "$CURRENT/source/WebUI"
        if [[ "$PKG" == *.zip ]]; then
            unzip -q -o "$PKG" -d "$CURRENT/source/WebUI"
            # 处理外层嵌套 WebUI 文件夹的情况
            if [[ -d "$CURRENT/source/WebUI/WebUI" ]]; then
                mv "$CURRENT/source/WebUI/WebUI/"* "$CURRENT/source/WebUI/"
                rm -rf "$CURRENT/source/WebUI/WebUI"
            fi
        else
            tar -zxvf "$PKG" -C "$CURRENT/source/WebUI" --strip-components=1 >/dev/null
        fi
    else
        local uem_pkg=$(get_uem_pkg)
        TMPDIR=$(mktemp -d /tmp/uem_tmp_XXXX)
        tar -zxvf "$uem_pkg" -C "$TMPDIR" >/dev/null
        local TMP_CONTENT=$(find "$TMPDIR" -mindepth 1 -maxdepth 1 -type d | head -n1)
        local WEBUI_TAR=$(find "$TMP_CONTENT" -maxdepth 1 -type f -name 'WebUI*.tar.gz' | head -n1)
        if [[ -z "$WEBUI_TAR" ]]; then log "UEM 包中未找到 WebUI 包"; rm -rf "$TMPDIR"; exit 1; fi
        rm -rf "$CURRENT/source/WebUI"
        mkdir -p "$CURRENT/source/WebUI"
        tar -zxvf "$WEBUI_TAR" -C "$CURRENT/source/WebUI" --strip-components=1 >/dev/null
        rm -rf "$TMPDIR"
    fi
    chmod -R 777 "$CURRENT/source/WebUI"
    log "WebUI 更新完成"
}

do_update_tomcat() {
    log "开始更新 Tomcat..."
    local PKG=""
    local TMPDIR=""
    if [[ $IS_LOCAL -eq 1 ]]; then
        PKG=$(find "$BASEDIR" -maxdepth 1 -type f -name 'tomcat*.tar.gz' | head -n1)
        if [[ -z "$PKG" ]]; then log "未找到本地 tomcat.tar.gz 包"; exit 1; fi
    else
        log "Tomcat 目前仅支持 local 模式更新 (需要 tomcat.tar.gz)"
        exit 1
    fi
    
    log "使用 Tomcat 包: $PKG"
    TMPDIR=$(mktemp -d /tmp/tomcat_tmp_XXXX)
    tar -zxvf "$PKG" -C "$TMPDIR" >/dev/null
    
    if [[ ! -d "$TMPDIR/tomcat" ]]; then
        log "错误: Tomcat 包解压后未发现 tomcat 目录！"
        rm -rf "$TMPDIR"
        exit 1
    fi
    
    systemctl stop tomcat 2>/dev/null || true
    
    # 备份旧的 WAR 包
    local BK_WEBAPPS=$(mktemp -d /tmp/webapps_tmp_XXXX)
    if [[ -d "$CURRENT/tomcat/webapps" ]]; then
        log "正在备份原有业务 WAR 包..."
        find "$CURRENT/tomcat/webapps" -maxdepth 1 -name "*.war" -exec cp -f {} "$BK_WEBAPPS/" \;
    fi
    
    log "正在清理旧 Tomcat 目录并部署新版本..."
    rm -rf "$CURRENT/tomcat"
    mv "$TMPDIR/tomcat" "$CURRENT/"
    
    # 恢复备份的 WAR 包
    if [[ "$(ls -A $BK_WEBAPPS 2>/dev/null)" ]]; then
        log "正在恢复业务 WAR 包..."
        mkdir -p "$CURRENT/tomcat/webapps"
        cp -f "$BK_WEBAPPS/"*.war "$CURRENT/tomcat/webapps/" 2>/dev/null || true
    fi
    
    rm -rf "$BK_WEBAPPS"
    rm -rf "$TMPDIR"
    chown -R tomcat:tomcat "$CURRENT/tomcat"
    chmod +x "$CURRENT/tomcat/bin/"*.sh
    systemctl start tomcat 2>/dev/null || true
    log "Tomcat 更新完成"
}

do_update_backend() {
    log "开始更新 Backend (WAR 包)..."
    local uem_pkg=$(get_uem_pkg)
    local TMPDIR=$(mktemp -d /tmp/uem_tmp_XXXX)
    tar -zxvf "$uem_pkg" -C "$TMPDIR" >/dev/null
    local TMP_CONTENT=$(find "$TMPDIR" -mindepth 1 -maxdepth 1 -type d | head -n1)
    
    local WAR_COUNT=$(find "$TMP_CONTENT" -maxdepth 1 -name "*.war" | wc -l)
    if [[ $WAR_COUNT -eq 0 ]]; then
        log "未在 UEM 包中找到任何 .war 文件"
        rm -rf "$TMPDIR"
        exit 1
    fi
    
    systemctl stop tomcat 2>/dev/null || true
    
    log "正在清理旧的 war 包及解压目录..."
    rm -rf "$CURRENT/tomcat/webapps/"*
    
    while IFS=  read -r -d $'\0'; do
        cp -f "$REPLY" "$CURRENT/tomcat/webapps/"
        log "已部署 $(basename "$REPLY")"
    done < <(find "$TMP_CONTENT" -maxdepth 1 -name "*.war" -print0)
    
    rm -rf "$TMPDIR"
    chown -R tomcat:tomcat "$CURRENT/tomcat/webapps"
    systemctl start tomcat 2>/dev/null || true
    log "Backend 更新完成"
}

do_update_module() {
    local mod=$1
    log "开始更新模块: $mod"
    local TAR_PREFIX=${MOD_MAP[$mod]:-$mod}
    local PKG=""
    local TMPDIR=""
    
    if [[ $IS_LOCAL -eq 1 ]]; then
        PKG=$(find "$BASEDIR" -maxdepth 1 -type f -name "${TAR_PREFIX}*.tar.gz" | head -n1)
        if [[ -z "$PKG" ]]; then log "未找到本地包: ${TAR_PREFIX}*.tar.gz"; exit 1; fi
        log "使用本地包: $PKG"
        rm -rf "$CURRENT/source/$mod"
        mkdir -p "$CURRENT/source/$mod"
        tar -zxvf "$PKG" -C "$CURRENT/source/$mod" --strip-components=1 >/dev/null
    else
        local uem_pkg=$(get_uem_pkg)
        TMPDIR=$(mktemp -d /tmp/uem_tmp_XXXX)
        tar -zxvf "$uem_pkg" -C "$TMPDIR" >/dev/null
        local TMP_CONTENT=$(find "$TMPDIR" -mindepth 1 -maxdepth 1 -type d | head -n1)
        PKG=$(find "$TMP_CONTENT" -maxdepth 1 -type f -name "${TAR_PREFIX}*.tar.gz" | head -n1)
        if [[ -z "$PKG" ]]; then log "UEM 包中未找到模块包: ${TAR_PREFIX}"; rm -rf "$TMPDIR"; exit 1; fi
        rm -rf "$CURRENT/source/$mod"
        mkdir -p "$CURRENT/source/$mod"
        tar -zxvf "$PKG" -C "$CURRENT/source/$mod" --strip-components=1 >/dev/null
        rm -rf "$TMPDIR"
    fi
    chown -R root:root "$CURRENT/source/$mod"
    chmod -R 777 "$CURRENT/source/$mod"
    
    # 使用统一管理命令重启服务
    log "正在重启服务..."
    if command -v cncymdm >/dev/null 2>&1; then
        cncymdm restart 2>/dev/null || log "cncymdm restart 执行失败，请手动重启"
    else
        log "未找到 cncymdm 命令，请您手动重启相关进程"
    fi
    log "模块 $mod 更新完成"
}

case "$TARGET" in
    uem) do_update_uem ;;
    webui) do_update_webui ;;
    tomcat) do_update_tomcat ;;
    backend) do_update_backend ;;
    *)
        if [[ -n "${MOD_MAP[$TARGET]:-}" ]] || [[ -d "$CURRENT/source/$TARGET" ]]; then
            do_update_module "$TARGET"
        else
            log "未知的组件名称: $TARGET"
            echo "支持的分项模块: ${!MOD_MAP[@]} 以及 source 下存在的目录"
            exit 1
        fi
        ;;
esac

exit 0
