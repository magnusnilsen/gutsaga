-- GutSaga quest log for Forever, retail Midnight, Classic Era, and Classic (Mists).
-- Interface numbers live in the .toc files (see addon/INTERFACE).
-- Completions are stored in GutSagaDB (SavedVariables). On Classic Era the
-- companion reads those after /reload or logout — live chat-log signaling is parked.
-- There is no completion button. Goals may complete in any order unless
-- objectives_ordered is true on the saga.

local PREFIX = "GutSaga"
local player = { name = "", realm = "", level = 1, race = "", class = "", battletag = "" }
local world = {
    zone = "", subzone = "", minimap = "", mapID = nil, x = nil, y = nil,
    npc = "", object = "", killName = nil, killSecret = false, deathName = nil, attests = {},
    gameHour = nil, gameMin = nil, instanceName = "", groupSnap = nil, pendingEmote = nil,
}
local readClocks
local readGroup
local frame
local toast
local showSagaToast = function() end
local sectionOpen
local sectionDone
local listPanel
local detailPanel
local GOLD = { r = 0.99, g = 0.93, b = 0.59 }
local LOCKED_TITLE = { r = 0.45, g = 0.42, b = 0.38 }
local LOCKED_SUB = { r = 0.40, g = 0.37, b = 0.34 }
-- Fallback difficulty colors when GetQuestDifficultyColor is missing.
local DIFF_YELLOW = { r = 1.00, g = 1.00, b = 0.00 }
local DIFF_RED = { r = 1.00, g = 0.10, b = 0.10 }
local DIFF_ORANGE = { r = 1.00, g = 0.50, b = 0.25 }
local DIFF_GREEN = { r = 0.25, g = 0.75, b = 0.25 }
local DIFF_GRAY = { r = 0.50, g = 0.50, b = 0.50 }
local listChild
local listScroll
local detailFS
local detailImage
local detailTitle
local detailMeta
local detailGoalsHeader
local detailActivityHeader
local detailChild
local detailScroll
local rows = {}
local goalRows = {}
local activityRows = {}
local INK = { r = 0.22, g = 0.12, b = 0.04 }
local INK_MUTED = { r = 0.42, g = 0.35, b = 0.25 }
local lastDetailID
local selectedID
local built = false
local checking = false
local lastCharacterLine = ""
local syncScrollBar
local scrollBy
local bindWheel
local wheelOnFrame
local wheelOnList
local wheelOnDetail

local function secret(v)
    if issecretvalue == nil or v == nil then
        return false
    end
    local ok, isSecret = pcall(issecretvalue, v)
    return ok and isSecret and true or false
end

local function plainString(v)
    if type(v) ~= "string" or secret(v) or v == "" then
        return nil
    end
    return v
end

local function plainNumber(v)
    if type(v) ~= "number" or secret(v) then
        return nil
    end
    return v
end

local function same(a, b)
    if type(a) ~= "string" or type(b) ~= "string" then
        return false
    end
    return a:lower() == b:lower()
end

local function norm(s)
    if type(s) ~= "string" then
        return ""
    end
    return (s:lower():gsub("^%s+", ""):gsub("%s+$", ""))
end

local function unixNow()
    if GetServerTime then
        local ok, v = pcall(GetServerTime)
        local n = ok and plainNumber(v) or nil
        if n and n > 0 then
            return math.floor(n)
        end
    end
    return time()
end

-- Machine COMPLETE/CHARACTER/LEVELUP lines are parked on Classic Era: chat
-- logging does not reliably accept addon SendChatMessage (taint / no flush).
-- Completions go to GutSagaDB and reach Discord after /reload via the companion.
local function chat(line)
    local machine = type(line) == "string" and line:find("[GutSaga]", 1, true)
        and (line:find(" COMPLETE ", 1, true) or line:find(" CHARACTER ", 1, true) or line:find(" LEVELUP ", 1, true))
    if machine then
        return
    end
    if DEFAULT_CHAT_FRAME and DEFAULT_CHAT_FRAME.AddMessage then
        DEFAULT_CHAT_FRAME:AddMessage(line)
    else
        print(line)
    end
end

local function registerPrefix(prefix)
    if C_ChatInfo and C_ChatInfo.RegisterAddonMessagePrefix then
        pcall(C_ChatInfo.RegisterAddonMessagePrefix, prefix)
    elseif RegisterAddonMessagePrefix then
        pcall(RegisterAddonMessagePrefix, prefix)
    end
end

local function sendAddon(prefix, message, channel)
    if C_ChatInfo and C_ChatInfo.SendAddonMessage then
        pcall(C_ChatInfo.SendAddonMessage, prefix, message, channel)
    elseif SendAddonMessage then
        pcall(SendAddonMessage, prefix, message, channel)
    end
end

local function initDB()
    if type(GutSagaDB) ~= "table" then
        GutSagaDB = {}
    end
    if type(GutSagaDB.completions) ~= "table" then
        GutSagaDB.completions = {}
    end
    if type(GutSagaDB.objectives) ~= "table" then
        GutSagaDB.objectives = {}
    end
    if type(GutSagaDB.pending_resets) ~= "table" then
        GutSagaDB.pending_resets = {}
    end
    if type(GutSagaDB.read) ~= "table" then
        GutSagaDB.read = {}
    end
end

local minimapBtn
local updateMinimapUnread

-- Queue an API progress wipe for the companion (after /reload writes SV).
-- Without this, the next sync restores completed status and local reset looks broken.
local function queueReset(questID)
    initDB()
    local row = { at = unixNow() }
    if questID and questID ~= "" then
        row.quest_id = questID
    else
        row.all = true
    end
    table.insert(GutSagaDB.pending_resets, row)
end

-- GutSagaData is only read at login and /reload. The companion writes notice
-- into that file when a saga changed, and shows the "skriv /reload" banner
-- on the desktop because it cannot type into the client. After /reload this
-- prints the notice once per character.
local function showFreshNotice()
    if type(GutSagaData) ~= "table" then
        return
    end
    local id = GutSagaData.notice_id
    local text = GutSagaData.notice
    if type(id) ~= "string" or id == "" or type(text) ~= "string" or text == "" then
        return
    end
    initDB()
    if GutSagaDB.seen_notice == id then
        return
    end
    GutSagaDB.seen_notice = id
    chat("|cffc9a227GutSaga|r: " .. text)
end

local function pipe(s)
    s = s or ""
    s = s:gsub("|", "/")
    s = s:gsub("%s+", " ")
    s = (s:gsub("^%s+", ""):gsub("%s+$", ""))
    if s == "" then
        return "-"
    end
    return s
end

local function readBattleTag()
    if not BNGetInfo then
        return ""
    end
    local results = { pcall(BNGetInfo) }
    if not results[1] then
        return ""
    end
    for i = 2, #results do
        local s = plainString(results[i])
        if s and s:find("#", 1, true) then
            return s
        end
    end
    return ""
end

local function unitLabel(fn)
    if not fn then
        return ""
    end
    local ok, localized, english = pcall(fn, "player")
    if not ok then
        return ""
    end
    local eng = plainString(english)
    if eng then
        return eng
    end
    return plainString(localized) or ""
end

local function cachePlayer()
    if InCombatLockdown and InCombatLockdown() then
        return
    end
    if UnitFullName then
        local ok, name, realm = pcall(UnitFullName, "player")
        if ok then
            local n = plainString(name)
            if n then
                player.name = n
            end
            local r = plainString(realm)
            if r then
                player.realm = r
            end
        end
    end
    if player.realm == "" and GetRealmName then
        local ok, realm = pcall(GetRealmName)
        if ok then
            local r = plainString(realm)
            if r then
                player.realm = r
            end
        end
    end
    if UnitLevel then
        local ok, level = pcall(UnitLevel, "player")
        if ok then
            local n = plainNumber(level)
            if n and n > 0 then
                player.level = n
            end
        end
    end
    local race = unitLabel(UnitRace)
    if race ~= "" then
        player.race = race
    end
    local className = unitLabel(UnitClass)
    if className ~= "" then
        player.class = className
    end
    local tag = readBattleTag()
    if tag ~= "" then
        player.battletag = tag
    end
end

local function emitCharacter()
    if player.name == "" or player.level < 1 then
        return
    end
    local line = string.format(
        "[GutSaga] CHARACTER %s|%s|%d|%s|%s|%s|%d",
        pipe(player.name), pipe(player.realm), player.level,
        pipe(player.race), pipe(player.class), pipe(player.battletag), unixNow()
    )
    if line == lastCharacterLine then
        return
    end
    lastCharacterLine = line
    chat(line)
end

local function isComplete(entry)
    if type(entry) ~= "table" then
        return false
    end
    if entry.status == "completed" then
        return true
    end
    local id = entry.quest_id
    return type(id) == "string" and type(GutSagaDB) == "table"
        and type(GutSagaDB.completions) == "table" and GutSagaDB.completions[id] ~= nil
end

-- WoW-style quest-log colors from saga min_level vs player level.
-- No min_level → yellow. Far below → red, a bit below → orange,
-- around / slight above → yellow, a bit above → green, a lot above → gray.
local function difficultyColor(minLevel, playerLevel)
    local level = tonumber(minLevel)
    if not level or level < 1 then
        return DIFF_YELLOW
    end
    local pl = tonumber(playerLevel) or 1
    if GetQuestDifficultyColor then
        local ok, c = pcall(GetQuestDifficultyColor, level)
        if ok and type(c) == "table" and type(c.r) == "number" then
            return c
        end
    end
    local diff = level - pl
    if diff >= 5 then
        return DIFF_RED
    elseif diff >= 3 then
        return DIFF_ORANGE
    elseif diff >= -2 then
        return DIFF_YELLOW
    end
    local greenRange = 5
    if GetQuestGreenRange then
        local ok, n = pcall(GetQuestGreenRange)
        if ok and type(n) == "number" and n > 0 then
            greenRange = n
        end
    end
    if -diff <= greenRange then
        return DIFF_GREEN
    end
    return DIFF_GRAY
end

local function currentEntries()
    local out = {}
    if player.name == "" or type(GutSagaData) ~= "table" or type(GutSagaData.entries) ~= "table" then
        return out
    end
    for _, entry in ipairs(GutSagaData.entries) do
        if type(entry) == "table" and same(entry.character, player.name) then
            local realm = entry.realm
            if type(realm) ~= "string" or realm == "" or player.realm == "" or same(realm, player.realm) then
                table.insert(out, entry)
            end
        end
    end
    table.sort(out, function(a, b)
        local ac = isComplete(a) and 1 or 0
        local bc = isComplete(b) and 1 or 0
        if ac ~= bc then
            return ac < bc
        end
        local achain = (type(a.chain_id) == "string" and a.chain_id ~= "") and 0 or 1
        local bchain = (type(b.chain_id) == "string" and b.chain_id ~= "") and 0 or 1
        if achain ~= bchain then
            return achain < bchain
        end
        local an = a.chain_name or a.chain_id or ""
        local bn = b.chain_name or b.chain_id or ""
        if an ~= bn then
            return an < bn
        end
        local ao = tonumber(a.chain_order) or 0
        local bo = tonumber(b.chain_order) or 0
        if ao ~= bo then
            return ao < bo
        end
        local am = tonumber(a.min_level) or 0
        local bm = tonumber(b.min_level) or 0
        if am ~= bm then
            return am < bm
        end
        return (a.title or "") < (b.title or "")
    end)
    return out
end

local function findEntry(id)
    local entries = currentEntries()
    for i = 1, #entries do
        if entries[i].quest_id == id then
            return entries[i]
        end
    end
    return nil
end

-- GutSagaDB is SavedVariablesPerCharacter, so read state is already per toon.
-- Flat map: read[quest_id] = true. read_bootstrapped means the one-time
-- upgrade seed already ran. Incomplete sagas missing from read are unread.
local function flattenReadMap(raw)
    local flat = {}
    if type(raw) ~= "table" then
        return flat
    end
    for k, v in pairs(raw) do
        if type(v) == "table" then
            -- Old name\nrealm nesting from the first unread build.
            for qid, flag in pairs(v) do
                if flag and type(qid) == "string" then
                    flat[qid] = true
                end
            end
        elseif v == true and type(k) == "string" then
            flat[k] = true
        end
    end
    return flat
end

local function ensureReadMap()
    initDB()
    if GutSagaDB.read_bootstrapped then
        GutSagaDB.read = flattenReadMap(GutSagaDB.read)
        return GutSagaDB.read
    end

    local flat = flattenReadMap(GutSagaDB.read)
    -- First run: seed every saga already in the log as read so an upgrade is
    -- not a wall of blue. /gutsaga wipe|unread clears this for playtesting.
    local entries = currentEntries()
    for i = 1, #entries do
        local id = entries[i].quest_id
        if type(id) == "string" and id ~= "" then
            flat[id] = true
        end
    end
    GutSagaDB.read = flat
    GutSagaDB.read_bootstrapped = true
    return GutSagaDB.read
end

local function markRead(questID)
    if type(questID) ~= "string" or questID == "" then
        return
    end
    local bucket = ensureReadMap()
    bucket[questID] = true
end

-- Mark every incomplete saga unread (and drop completed ids from the map).
local function markAllUnread()
    initDB()
    GutSagaDB.read = {}
    GutSagaDB.read_bootstrapped = true
    local entries = currentEntries()
    for i = 1, #entries do
        local e = entries[i]
        if isComplete(e) and type(e.quest_id) == "string" then
            GutSagaDB.read[e.quest_id] = true
        end
    end
end

local function isUnread(questID, entry)
    if type(questID) ~= "string" or questID == "" then
        return false
    end
    if entry and isComplete(entry) then
        return false
    end
    local bucket = ensureReadMap()
    return bucket[questID] ~= true
end

local function countUnreadSagas()
    local bucket = ensureReadMap()
    local entries = currentEntries()
    local n = 0
    for i = 1, #entries do
        local e = entries[i]
        if not isComplete(e) and type(e.quest_id) == "string" and bucket[e.quest_id] ~= true then
            n = n + 1
        end
    end
    return n
end

local function hasUnreadSagas()
    return countUnreadSagas() > 0
end

local function unreadTooltipLine()
    local n = countUnreadSagas()
    if n <= 0 then
        return nil
    end
    if n == 1 then
        return "1 ny saga"
    end
    return n .. " nye sagaer"
end

-- Medium sky blue from unread.tga (● is missing in the default WoW font).
local UNREAD_BLUE = { r = 0.38, g = 0.68, b = 1.0 }
-- Always leave room for the list dot so rows do not jump when marked read.
local LIST_TEXT_INSET = 20
local function paintUnreadDot(tex)
    if not tex then
        return
    end
    tex:SetTexture("Interface\\AddOns\\GutSaga\\unread")
    tex:SetVertexColor(UNREAD_BLUE.r, UNREAD_BLUE.g, UNREAD_BLUE.b)
    if tex.SetAlpha then
        tex:SetAlpha(1)
    end
end

local function layoutListRow(row, unread)
    row.title:ClearAllPoints()
    row.title:SetPoint("TOPLEFT", LIST_TEXT_INSET, -4)
    row.title:SetPoint("TOPRIGHT", -8, -4)
    row.sub:ClearAllPoints()
    row.sub:SetPoint("BOTTOMLEFT", LIST_TEXT_INSET, 4)
    row.sub:SetPoint("BOTTOMRIGHT", -8, 4)
    if unread then
        row.unread:ClearAllPoints()
        -- Sit beside the title line, not vertically centered on the whole row.
        row.unread:SetPoint("TOPLEFT", 5, -6)
        row.unread:Show()
    else
        row.unread:Hide()
    end
end

updateMinimapUnread = function()
    if not minimapBtn or not minimapBtn.unreadGlow then
        return
    end
    if hasUnreadSagas() then
        minimapBtn.unreadGlow:Show()
    else
        minimapBtn.unreadGlow:Hide()
    end
end

local function formatDescription(text)
    if type(text) ~= "string" or text == "" then
        return "Ingen"
    end
    text = text:gsub("\r\n", "\n"):gsub("\r", "\n")
    text = text:gsub("\\n", "\n")
    text = text:gsub("[ \t]+\n", "\n"):gsub("\n[ \t]+", "\n")
    text = text:match("^%s*(.-)%s*$") or text
    if text == "" then
        return "Ingen"
    end
    return text
end

local NO_MONTHS = {
    "januar", "februar", "mars", "april", "mai", "juni",
    "juli", "august", "september", "oktober", "november", "desember",
}

local function isLeapYear(y)
    return (y % 4 == 0 and y % 100 ~= 0) or (y % 400 == 0)
end

local function utcUnix(y, m, d, hh, mm, ss)
    local mdays = { 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31 }
    local days = 0
    for yr = 1970, y - 1 do
        days = days + (isLeapYear(yr) and 366 or 365)
    end
    for mo = 1, m - 1 do
        local dim = mdays[mo]
        if mo == 2 and isLeapYear(y) then
            dim = 29
        end
        days = days + dim
    end
    days = days + d - 1
    return days * 86400 + hh * 3600 + mm * 60 + ss
end

local function lastSundayUTC(year, month)
    local mdays = { 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31 }
    local dim = mdays[month]
    if month == 2 and isLeapYear(year) then
        dim = 29
    end
    for d = dim, dim - 6, -1 do
        local t = date("!*t", utcUnix(year, month, d, 12, 0, 0))
        if t and t.wday == 1 then
            return d
        end
    end
    return dim
end

-- Europe/Oslo: last Sunday of March 01:00 UTC → CEST; last Sunday of October 01:00 UTC → CET.
local function osloOffset(sec)
    local t = date("!*t", sec)
    if not t then
        return 3600
    end
    local start = utcUnix(t.year, 3, lastSundayUTC(t.year, 3), 1, 0, 0)
    local finish = utcUnix(t.year, 10, lastSundayUTC(t.year, 10), 1, 0, 0)
    if sec >= start and sec < finish then
        return 7200
    end
    return 3600
end

local function formatOsloWhen(raw)
    if type(raw) == "string" and raw ~= "" and not tonumber(raw) then
        -- Companion already formatted "dd mmmm yyyy hh:mm".
        if raw:find("%d%d %a+ %d%d%d%d %d%d:%d%d") then
            return raw
        end
    end
    local sec = plainNumber(raw)
    if not sec and type(raw) == "string" then
        local y, mo, d, hh, mm, ss = raw:match("^(%d%d%d%d)%-(%d%d)%-(%d%d)T(%d%d):(%d%d):(%d%d)")
        if y then
            sec = utcUnix(tonumber(y), tonumber(mo), tonumber(d), tonumber(hh), tonumber(mm), tonumber(ss) or 0)
        end
    end
    if not sec or sec < 1 then
        return ""
    end
    sec = math.floor(sec)
    local localSec = sec + osloOffset(sec)
    local t = date("!*t", localSec)
    if not t then
        return ""
    end
    local month = NO_MONTHS[t.month] or ""
    return string.format("%02d %s %d %02d:%02d", t.day, month, t.year, t.hour, t.min)
end

-- English class tokens for RAID_CLASS_COLORS (stored race/class are English from Unit*).
local function classToken(className)
    local s = norm(className):gsub("%s+", "")
    if s == "" then
        return ""
    end
    local aliases = {
        deathknight = "DEATHKNIGHT",
        demonhunter = "DEMONHUNTER",
        kriger = "WARRIOR",
        paladin = "PALADIN",
        jeger = "HUNTER",
        slyngel = "ROGUE",
        prest = "PRIEST",
        sjaman = "SHAMAN",
        magiker = "MAGE",
        hekser = "WARLOCK",
        druide = "DRUID",
    }
    return aliases[s] or string.upper(s)
end

local function classRGB(className)
    local token = classToken(className)
    if token ~= "" and RAID_CLASS_COLORS and RAID_CLASS_COLORS[token] then
        local c = RAID_CLASS_COLORS[token]
        return c.r or 1, c.g or 1, c.b or 1
    end
    local fallback = {
        WARRIOR = { 0.78, 0.61, 0.43 },
        PALADIN = { 0.96, 0.55, 0.73 },
        HUNTER = { 0.67, 0.83, 0.45 },
        ROGUE = { 1.00, 0.96, 0.41 },
        PRIEST = { 1.00, 1.00, 1.00 },
        SHAMAN = { 0.00, 0.44, 0.87 },
        MAGE = { 0.41, 0.80, 0.94 },
        WARLOCK = { 0.58, 0.51, 0.79 },
        DRUID = { 1.00, 0.49, 0.04 },
        DEATHKNIGHT = { 0.77, 0.12, 0.23 },
        MONK = { 0.00, 1.00, 0.59 },
        DEMONHUNTER = { 0.64, 0.19, 0.79 },
        EVOKER = { 0.20, 0.58, 0.50 },
    }
    local c = fallback[token]
    if c then
        return c[1], c[2], c[3]
    end
    return INK.r, INK.g, INK.b
end

local function activityList(entry)
    local out = {}
    local seen = {}
    local function add(row)
        if type(row) ~= "table" then
            return
        end
        local character = plainString(row.character) or ""
        if character == "" then
            return
        end
        local realm = plainString(row.realm) or ""
        local key = (character .. "\n" .. realm):lower()
        if seen[key] then
            return
        end
        seen[key] = true
        local at = ""
        if type(row.completed_at) == "string" then
            at = row.completed_at
        elseif type(row.completed_at) == "number" then
            at = string.format("%014d", row.completed_at)
        end
        local when = plainString(row["when"]) or ""
        if when == "" then
            when = formatOsloWhen(row.completed_at)
        end
        table.insert(out, {
            when = when,
            character = character,
            battletag = plainString(row.battletag) or "",
            level = tonumber(row.level) or 0,
            race = plainString(row.race) or "",
            class = plainString(row.class) or "",
            at = at,
        })
    end
    if type(entry) == "table" and type(entry.completed_by) == "table" then
        for i = 1, #entry.completed_by do
            add(entry.completed_by[i])
        end
    end
    -- Local finishes before the companion has synced them to the API.
    if type(entry) == "table" and type(entry.quest_id) == "string" then
        initDB()
        local localDone = GutSagaDB.completions and GutSagaDB.completions[entry.quest_id]
        if type(localDone) == "table" then
            add({
                character = localDone.character or player.name,
                realm = localDone.realm or player.realm,
                battletag = player.battletag,
                level = localDone.level or player.level,
                race = localDone.race or player.race,
                class = localDone.class or player.class,
                completed_at = localDone.completed_at,
            })
        end
    end
    table.sort(out, function(a, b)
        if a.at ~= b.at then
            return a.at < b.at
        end
        return a.character < b.character
    end)
    return out
end

local function savedSteps(entry)
    initDB()
    local version = tonumber(entry.quest_version) or 0
    local bucket = GutSagaDB.objectives[entry.quest_id]
    if type(bucket) ~= "table" or tonumber(bucket.version) ~= version then
        bucket = { version = version, steps = {} }
        GutSagaDB.objectives[entry.quest_id] = bucket
    end
    if type(bucket.steps) ~= "table" then
        bucket.steps = {}
    end
    return bucket
end

local function itemQualityHex(quality)
    local row = ITEM_QUALITY_COLORS and ITEM_QUALITY_COLORS[quality]
    if type(row) == "table" and type(row.hex) == "string" and row.hex ~= "" then
        return row.hex
    end
    local fallback = {
        [0] = "|cff9d9d9d",
        [1] = "|cffffffff",
        [2] = "|cff1eff00",
        [3] = "|cff0070dd",
        [4] = "|cffa335ee",
        [5] = "|cffff8000",
        [6] = "|cffe6cc80",
        [7] = "|cff00ccff",
    }
    return fallback[quality] or "|cffffffff"
end

local function itemLink(id, fallbackName)
    local name, link, quality
    if GetItemInfo then
        local ok, a, b, c = pcall(GetItemInfo, id)
        if ok then
            name, link, quality = a, b, c
        end
    end
    if (type(name) ~= "string" or name == "") and C_Item and C_Item.GetItemInfo then
        local ok, a, b, c = pcall(C_Item.GetItemInfo, id)
        if ok then
            name, link, quality = a, b, c
        end
    end
    if type(name) ~= "string" or name == "" then
        if C_Item and C_Item.RequestLoadItemDataByID then
            pcall(C_Item.RequestLoadItemDataByID, id)
        end
        if type(fallbackName) == "string" and fallbackName ~= "" then
            return "|cffffffff[" .. fallbackName .. "]|r"
        end
        return ""
    end
    if type(link) == "string" and string.find(link, "|H", 1, true) then
        return link
    end
    return itemQualityHex(quality) .. "[" .. name .. "]|r"
end

local function applyItemLink(text, obj)
    local id = tonumber(obj.item_id)
    if not id then
        return text
    end
    local link = itemLink(id, obj.item_name)
    if link == "" then
        return text
    end
    local plain = obj.item_name
    if type(plain) == "string" and plain ~= "" then
        local i, j = string.find(text, plain, 1, true)
        if i then
            return string.sub(text, 1, i - 1) .. link .. string.sub(text, j + 1)
        end
    end
    return text .. " " .. link
end

local function creatorName(entry)
    if type(entry.created_by) == "string" and entry.created_by ~= "" then
        return entry.created_by
    end
    return "Ukjent"
end

local function completionSet()
    local done = {}
    if type(GutSagaDB) == "table" and type(GutSagaDB.completions) == "table" then
        for id in pairs(GutSagaDB.completions) do
            if type(id) == "string" then
                done[id] = true
            end
        end
    end
    return done
end

local function entryLocked(entry, entries)
    if type(entry) ~= "table" or type(GutSagaChains) ~= "table" or type(GutSagaChains.isLocked) ~= "function" then
        return false
    end
    return GutSagaChains.isLocked(entry, entries, completionSet()) and true or false
end

local function listSubtitle(entry, entries)
    local fallback = creatorName(entry)
    if type(GutSagaChains) ~= "table" or type(GutSagaChains.listSub) ~= "function" then
        return fallback
    end
    return GutSagaChains.listSub(entry, entries, completionSet(), fallback)
end

local function sectionExpanded(key, defaultOpen)
    initDB()
    if type(GutSagaDB.sections) ~= "table" then
        GutSagaDB.sections = {}
    end
    local v = GutSagaDB.sections[key]
    if v == nil then
        return defaultOpen
    end
    return v and true or false
end

local function setSectionExpanded(key, open)
    initDB()
    if type(GutSagaDB.sections) ~= "table" then
        GutSagaDB.sections = {}
    end
    GutSagaDB.sections[key] = open and true or false
end

readClocks = function()
    local clocks = {}
    if GetGameTime then
        local ok, h, m = pcall(GetGameTime)
        h = ok and plainNumber(h) or nil
        m = ok and plainNumber(m) or nil
        if h and m then
            world.gameHour, world.gameMin = h, m
        end
    end
    if world.gameHour and world.gameMin then
        clocks.game = { hour = world.gameHour, min = world.gameMin }
    end
    if date and time then
        local ok, t = pcall(date, "*t")
        local unix = plainNumber(time())
        if ok and type(t) == "table" then
            local h, m = plainNumber(t.hour), plainNumber(t.min)
            if h and m then
                clocks["local"] = { hour = h, min = m, unix = unix }
            end
        end
    end
    if GetServerTime and date then
        local ok, server = pcall(GetServerTime)
        server = ok and plainNumber(server) or nil
        if server and server > 0 then
            local okT, t = pcall(date, "!*t", server)
            if okT and type(t) == "table" then
                local h, m = plainNumber(t.hour), plainNumber(t.min)
                if h and m then
                    clocks.realm = { hour = h, min = m, unix = server }
                end
            end
        end
    end
    return clocks
end

local function readInstance()
    if not GetInstanceInfo then
        return world.instanceName or ""
    end
    local ok, name, instanceType = pcall(GetInstanceInfo)
    if not ok or secret(name) or secret(instanceType) then
        return world.instanceName or ""
    end
    local typ = plainString(instanceType)
    local label = plainString(name)
    if not typ or typ == "none" or not label then
        world.instanceName = ""
        return ""
    end
    world.instanceName = label
    return label
end

readGroup = function()
    local members, seen, blocked = {}, {}, false
    local function add(unit, fallbackName, fallbackRealm, fallbackLevel)
        local name, realm, level
        if UnitName then
            local ok, n, r = pcall(UnitName, unit)
            if ok and secret(n) then
                blocked = true
                return
            end
            name = ok and plainString(n) or nil
            realm = ok and plainString(r) or nil
        end
        if (not name or name == "") and fallbackName and fallbackName ~= "" then
            name = fallbackName
            realm = fallbackRealm
        end
        if not name or name == "" then
            return
        end
        if UnitLevel then
            local okL, lvl = pcall(UnitLevel, unit)
            if okL and secret(lvl) then
                blocked = true
            else
                level = okL and plainNumber(lvl) or nil
            end
        end
        if (not level or level < 1) and fallbackLevel and fallbackLevel > 0 then
            level = fallbackLevel
        end
        local id = norm(name)
        if seen[id] then
            return
        end
        seen[id] = true
        if not realm or realm == "" then
            realm = player.realm or ""
        end
        table.insert(members, { name = name, realm = realm, level = level or 0 })
    end
    add("player", player.name, player.realm, player.level)
    local grouped = false
    if IsInGroup then
        local ok, v = pcall(IsInGroup)
        grouped = ok and v == true and not secret(v)
    end
    if grouped then
        local raid = false
        if IsInRaid then
            local ok, v = pcall(IsInRaid)
            raid = ok and v == true and not secret(v)
        end
        local count, includesPlayer = 0, true
        if GetNumGroupMembers then
            local ok, v = pcall(GetNumGroupMembers)
            local n = ok and plainNumber(v) or nil
            if n and n > 0 then
                count = n
            end
        end
        if count == 0 and raid and GetNumRaidMembers then
            local ok, v = pcall(GetNumRaidMembers)
            local n = ok and plainNumber(v) or nil
            if n then
                count = n
            end
        end
        if count == 0 and GetNumPartyMembers then
            local ok, v = pcall(GetNumPartyMembers)
            local n = ok and plainNumber(v) or nil
            if n then
                count, includesPlayer = n, false
            end
        end
        local prefix = raid and "raid" or "party"
        local slots = count
        if not raid and includesPlayer then
            slots = count - 1
        end
        if slots < 0 then
            slots = 0
        elseif slots > 40 then
            slots = 40
        end
        for i = 1, slots do
            add(prefix .. i)
            if blocked then
                break
            end
        end
    end
    if not blocked then
        world.groupSnap = members
    end
    local snap = world.groupSnap
    if type(snap) ~= "table" or #snap == 0 then
        snap = { { name = player.name, realm = player.realm or "", level = player.level or 1 } }
    end
    return { instance = readInstance(), zone = world.zone, members = snap }
end

local function clockState()
    return { time = readClocks() }
end

local function timeHint(when, done, clocks)
    if type(when) ~= "table" then
        return ""
    end
    local bits = {}
    if type(when.start) == "string" and when.start ~= "" and type(when["end"]) == "string" and when["end"] ~= "" then
        table.insert(bits, when.start .. "–" .. when["end"])
    end
    if type(when.starts_at) == "string" and when.starts_at ~= "" then
        table.insert(bits, "fra " .. when.starts_at)
    end
    if type(when.ends_at) == "string" and when.ends_at ~= "" then
        table.insert(bits, "til " .. when.ends_at)
    end
    if #bits == 0 then
        return ""
    end
    local clock = when.clock or "local"
    local name = "lokal tid"
    if clock == "game" then
        name = "spilltid"
    elseif clock == "realm" then
        name = "realm-tid (UTC)"
    end
    local text = " (" .. table.concat(bits, ", ") .. ", " .. name .. ")"
    if not done and GutSagaObjectives and GutSagaObjectives.timeOpen and not GutSagaObjectives.timeOpen(when, clocks) then
        return " — venter" .. text
    end
    return text
end

local function objectiveLines(entry)
    local objs = entry.objectives
    local lines = {}
    if type(objs) ~= "table" then
        local text = type(objs) == "string" and objs or ""
        if text == "" then
            return lines
        end
        for line in string.gmatch(text, "[^\n]+") do
            table.insert(lines, { done = false, text = line })
        end
        return lines
    end
    local bucket = savedSteps(entry)
    local clocks = clockState()
    local function addObj(obj, index, depth)
        if type(obj) ~= "table" then
            return
        end
        local id = obj.id or tostring(index)
        local step = bucket.steps[id] or {}
        local done = step.done and true or false
        local pad = ""
        if depth > 0 then
            pad = string.rep("  ", depth) .. "- "
        end
        local mem = obj.memory
        if mem ~= "visit" and mem ~= "hold" then
            if obj.type == "item" or obj.type == "collect" then
                mem = "hold"
            else
                mem = "visit"
            end
        end
        local label
        if done and mem == "hold" then
            label = pad .. (obj.text or "Mål") .. " (nå)"
        elseif done then
            label = pad .. (obj.text or "Mål") .. " (Fullført)"
        else
            label = pad .. applyItemLink(obj.text or "Mål", obj)
            if obj.type == "location" or obj.type == "go" then
                local where = world.minimap
                if where == "" then
                    where = world.subzone
                end
                if where == "" then
                    where = world.zone
                end
                if where ~= "" then
                    label = label .. " (" .. where .. ")"
                end
            end
            local groupedKill = obj.type == "kill" and type(obj.group) == "table" and obj.group.level
            if obj.type == "collect" or obj.type == "item" or (obj.type == "kill" and not groupedKill) then
                local need = tonumber(obj.count) or 1
                local have = tonumber(step.count) or 0
                if obj.type == "collect" or obj.type == "item" then
                    local itemID = tonumber(obj.item_id)
                    if itemID then
                        local live = itemCount(itemID)
                        if live > have then
                            have = live
                        end
                    end
                end
                label = string.format("%s (%d/%d)", label, have, need)
            end
            if obj.type == "all_of" or obj.type == "any_of" then
                local need = tonumber(step.need) or (type(obj.objectives) == "table" and #obj.objectives or 0)
                local have = tonumber(step.count) or 0
                label = string.format("%s (%d/%d)", label, have, need)
            end
            if obj.type == "emote" and type(obj.emote) == "string" and obj.emote ~= "" then
                label = label .. " (/" .. string.lower(obj.emote) .. ")"
            end
            if obj.type == "sequence" and step.failed then
                label = label .. " (feilet)"
            end
            if obj.type == "kill" and type(obj.group) == "table" and obj.group.level then
                local roster = readGroup and readGroup() or {}
                local have, total = 0, 0
                local level = tonumber(obj.group.level) or 0
                for _, m in ipairs(roster.members or {}) do
                    if type(m) == "table" and type(m.name) == "string" and m.name ~= "" then
                        total = total + 1
                        if (tonumber(m.level) or 0) == level then
                            have = have + 1
                        end
                    end
                end
                local need = tonumber(obj.group.size) or total
                label = label .. string.format(" (%d/%d nivå %d)", have, need, level)
            end
            label = label .. timeHint(obj.when, done, clocks)
        end
        table.insert(lines, { done = done, failed = (not done) and step.failed and true or false, text = label })
        if obj.type == "sequence" and type(obj.beats) == "table" then
            local beat = tonumber(step.beat) or 1
            local failed = (not done) and step.failed and true or false
            local remain = tonumber(step.remain) or 0
            for i, b in ipairs(obj.beats) do
                if type(b) == "table" then
                    local beatDone = (not failed) and (done or i < beat)
                    local current = (not failed) and (not done) and i == beat
                    local beatLabel = b.text or "Del"
                    if current and remain > 0 then
                        beatLabel = beatLabel .. string.format(" (%ds)", remain)
                    end
                    table.insert(lines, {
                        done = beatDone,
                        failed = failed,
                        indent = 1,
                        text = beatLabel,
                    })
                end
            end
        elseif type(obj.objectives) == "table" then
            for i, child in ipairs(obj.objectives) do
                addObj(child, i, depth + 1)
            end
        end
    end
    for i, obj in ipairs(objs) do
        addObj(obj, i, 0)
    end
    if type(entry.when) == "table" and GutSagaObjectives and GutSagaObjectives.timeOpen then
        local open = GutSagaObjectives.timeOpen(entry.when, clocks)
        table.insert(lines, { done = open, text = "Tid" .. timeHint(entry.when, open, clocks) })
    end
    if type(entry.group) == "table" and tonumber(entry.group.size) and readGroup and GutSagaObjectives and GutSagaObjectives.groupOK then
        local group = readGroup()
        local minLevel = tonumber(entry.group.min_level) or 1
        local have = 0
        for _, m in ipairs(group.members or {}) do
            if type(m) == "table" and (tonumber(m.level) or 0) >= minLevel and type(m.name) == "string" and m.name ~= "" then
                have = have + 1
            end
        end
        local label = string.format("Sammen med gruppa (%d/%d)", have, tonumber(entry.group.size))
        if entry.group.min_level then
            label = label .. string.format(", nivå %d+", minLevel)
        end
        if type(entry.group.instance) == "string" and entry.group.instance ~= "" then
            label = label .. " i " .. entry.group.instance
        end
        table.insert(lines, { done = GutSagaObjectives.groupOK(entry.group, group), text = label })
    end
    return lines
end

local function inInstance()
    if not IsInInstance then
        return false
    end
    local ok, inside = pcall(IsInInstance)
    if not ok or secret(inside) then
        return true
    end
    return inside and true or false
end

local function broadcast(message)
    if inInstance() then
        return
    end
    if not ((C_ChatInfo and C_ChatInfo.SendAddonMessage) or SendAddonMessage) then
        return
    end
    local inRaid, inGroup, inGuild = false, false, false
    pcall(function()
        inRaid = IsInRaid() and true or false
    end)
    pcall(function()
        inGroup = IsInGroup() and true or false
    end)
    pcall(function()
        inGuild = IsInGuild() and true or false
    end)
    if inRaid then
        sendAddon(PREFIX, message, "RAID")
    elseif inGroup then
        sendAddon(PREFIX, message, "PARTY")
    end
    if inGuild then
        sendAddon(PREFIX, message, "GUILD")
    end
end

local function bagItemCount(id)
    local slotsOf = (C_Container and C_Container.GetContainerNumSlots) or GetContainerNumSlots
    local idOf = (C_Container and C_Container.GetContainerItemID) or GetContainerItemID
    local infoOf = (C_Container and C_Container.GetContainerItemInfo) or GetContainerItemInfo
    if type(slotsOf) ~= "function" then
        return nil
    end
    local total, found = 0, false
    for bag = 0, 4 do
        local okSlots, slots = pcall(slotsOf, bag)
        local nSlots = okSlots and plainNumber(slots) or 0
        for slot = 1, nSlots or 0 do
            local itemID, count
            if type(idOf) == "function" then
                local okID, got = pcall(idOf, bag, slot)
                itemID = okID and plainNumber(got) or nil
            end
            if type(infoOf) == "function" then
                local okInfo, a, b = pcall(infoOf, bag, slot)
                if okInfo and type(a) == "table" then
                    itemID = itemID or plainNumber(a.itemID)
                    count = plainNumber(a.stackCount) or 1
                elseif okInfo then
                    count = plainNumber(b) or 1
                end
            end
            if itemID == id then
                found = true
                total = total + (count or 1)
            end
        end
    end
    if not found then
        return nil
    end
    return total
end

itemCount = function(id)
    local best = 0
    local function consider(n)
        local v = plainNumber(n)
        if v and v > best then
            best = v
        end
    end
    if C_Item and C_Item.GetItemCount then
        local ok, n = pcall(C_Item.GetItemCount, id)
        if ok then
            consider(n)
        end
    end
    if GetItemCount then
        local ok, n = pcall(GetItemCount, id)
        if ok then
            consider(n)
        end
    end
    consider(bagItemCount(id))
    return best
end

local function readLocation()
    if InCombatLockdown and InCombatLockdown() then
        return
    end
    if GetZoneText then
        local ok, v = pcall(GetZoneText)
        local s = ok and plainString(v) or nil
        if s then
            world.zone = s
        end
    end
    if GetSubZoneText then
        local ok, v = pcall(GetSubZoneText)
        local s = ok and plainString(v) or nil
        if s then
            world.subzone = s
        end
    end
    if GetMinimapZoneText then
        local ok, v = pcall(GetMinimapZoneText)
        local s = ok and plainString(v) or nil
        if s then
            world.minimap = s
        end
    end
    local id
    if C_Map and C_Map.GetBestMapForUnit then
        local ok, mapID = pcall(C_Map.GetBestMapForUnit, "player")
        id = ok and plainNumber(mapID) or nil
    end
    if id then
        world.mapID = id
    end
    local function asNumber(v)
        if type(v) == "number" then
            return v
        end
        if type(v) == "string" then
            return tonumber(v)
        end
        return nil
    end
    -- Classic returns the position as a mixin object. That object is not
    -- always a plain table, so .x/.y and GetXY have to be read off it directly.
    local function xyOf(v, extra)
        local a, b = asNumber(v), asNumber(extra)
        if a and b then
            return a, b
        end
        if v == nil then
            return nil, nil
        end
        if type(v.GetXY) == "function" then
            local ok, px, py = pcall(v.GetXY, v)
            if ok then
                px, py = asNumber(px), asNumber(py)
                if px and py then
                    return px, py
                end
            end
        end
        local px, py = asNumber(v.x), asNumber(v.y)
        if px and py then
            return px, py
        end
        return nil, nil
    end
    local x, y
    if id and C_Map and C_Map.GetPlayerMapPosition then
        local posOK, a, b = pcall(C_Map.GetPlayerMapPosition, id, "player")
        if posOK then
            x, y = xyOf(a, b)
        end
    end
    if (not x or not y) and GetPlayerMapPosition then
        if SetMapToCurrentZone then
            pcall(SetMapToCurrentZone)
        end
        local posOK, a, b = pcall(GetPlayerMapPosition, "player")
        if posOK and type(a) == "number" and type(b) == "number" then
            x, y = a, b
        end
    end
    x = plainNumber(x)
    y = plainNumber(y)
    if x and y then
        world.x = x
        world.y = y
    end
end

local function readNPC()
    if not UnitName then
        return
    end
    local ok, name = pcall(UnitName, "npc")
    local s = ok and plainString(name) or nil
    if s then
        world.npc = s
        -- Keep the name briefly so a visit talk can latch even if the panel closes
        -- before the next full check.
        world.pendingTalk = { name = s, untilAt = unixNow() + 5 }
    end
end

local function clearNPC()
    world.npc = ""
end

local function readObject()
    if not UnitName then
        return
    end
    if UnitIsPlayer then
        local ok, isPlayer = pcall(UnitIsPlayer, "target")
        if ok and isPlayer == true and not secret(isPlayer) then
            return
        end
    end
    local ok, name = pcall(UnitName, "target")
    local s = ok and plainString(name) or nil
    if s then
        world.object = s
    end
end

local refresh
local finishQuest
local lastCheckError = ""

local function collectItems(obj, items)
    if type(obj) ~= "table" then
        return
    end
    if obj.type == "collect" or obj.type == "item" then
        local id = tonumber(obj.item_id)
        if id and items[id] == nil then
            items[id] = itemCount(id)
        end
    end
    if type(obj.objectives) == "table" then
        for _, child in ipairs(obj.objectives) do
            collectItems(child, items)
        end
    end
    if type(obj.beats) == "table" then
        for _, child in ipairs(obj.beats) do
            collectItems(child, items)
        end
    end
end

local function objectiveState(entries)
    local items = {}
    for _, entry in ipairs(entries) do
        if type(entry.objectives) == "table" and not isComplete(entry) then
            for _, obj in ipairs(entry.objectives) do
                collectItems(obj, items)
            end
        end
    end
    local emote, emoteTarget, emoteText, emoteZone, emoteSub, emoteMini, emoteMap, emoteX, emoteY
    local pending = world.pendingEmote
    if type(pending) == "table" and (tonumber(pending.untilAt) or 0) >= unixNow() then
        emote = pending.token
        emoteTarget = pending.target
        emoteText = pending.text
        emoteZone = pending.zone
        emoteSub = pending.subzone
        emoteMini = pending.minimap
        emoteMap = pending.mapID
        emoteX = pending.x
        emoteY = pending.y
    end
    local talkName = world.npc
    local pendingTalk = world.pendingTalk
    if (not talkName or talkName == "") and type(pendingTalk) == "table"
        and (tonumber(pendingTalk.untilAt) or 0) >= unixNow() then
        talkName = pendingTalk.name
    end
    return {
        zone = world.zone,
        subzone = world.subzone,
        minimap = world.minimap,
        map_id = world.mapID,
        x = world.x,
        y = world.y,
        npc = world.npc,
        talk_name = talkName,
        object = world.object,
        items = items,
        kill_name = world.killName,
        kill_secret = world.killSecret,
        emote = emote,
        emote_target = emoteTarget,
        emote_text = emoteText,
        emote_zone = emoteZone,
        emote_subzone = emoteSub,
        emote_minimap = emoteMini,
        emote_map_id = emoteMap,
        emote_x = emoteX,
        emote_y = emoteY,
        time = readClocks(),
    }
end

local function checkAll()
    if checking or type(GutSagaObjectives) ~= "table" or type(GutSagaObjectives.evaluate) ~= "function" then
        return
    end
    checking = true
    local ok, err = pcall(function()
        local entries = currentEntries()
        local state = objectiveState(entries)
        local group = readGroup()
        state.now = unixNow()
        state.group = group
        state.death_name = world.deathName
        for _, entry in ipairs(entries) do
            if not isComplete(entry) and not entryLocked(entry, entries) and type(entry.objectives) == "table" and #entry.objectives > 0 then
                local bucket = savedSteps(entry)
                local ordered = entry.objectives_ordered and true or false
                state.attests = world.attests[entry.quest_id]
                local steps, done = GutSagaObjectives.evaluate(entry.objectives, state, bucket.steps, ordered, entry.when)
                if state.emote and state.emote ~= "" then
                    for _, row in pairs(steps) do
                        if type(row) == "table" and row.consumed_emote then
                            world.pendingEmote = nil
                            break
                        end
                    end
                end
                bucket.steps = steps
                local groupReady = true
                if type(entry.group) == "table" and GutSagaObjectives.groupOK then
                    groupReady = GutSagaObjectives.groupOK(entry.group, group)
                end
                if done and groupReady then
                    finishQuest(entry, group)
                end
            end
        end
        world.killName = nil
        world.killSecret = false
        world.deathName = nil
    end)
    checking = false
    if not ok and err ~= lastCheckError then
        lastCheckError = err
        chat("[GutSaga] feil: " .. tostring(err))
    elseif ok then
        lastCheckError = ""
    end
end

local function onGoalLinkClick(_, link, text, button)
    if IsModifiedClick and IsModifiedClick("DRESSUP") and DressUpItemLink then
        pcall(DressUpItemLink, link)
        return
    end
    if SetItemRef then
        pcall(SetItemRef, link, text, button or "LeftButton")
    end
end

local function onGoalLinkEnter(owner, link)
    if not GameTooltip or type(link) ~= "string" then
        return
    end
    GameTooltip:SetOwner(owner, "ANCHOR_CURSOR")
    local ok = pcall(GameTooltip.SetHyperlink, GameTooltip, link)
    if ok then
        GameTooltip:Show()
    end
end

local function onGoalLinkLeave()
    if GameTooltip then
        GameTooltip:Hide()
    end
end

local function ensureGoalRow(i)
    local row = goalRows[i]
    if row then
        return row
    end
    row = CreateFrame("Frame", nil, detailChild)
    row:SetHeight(18)
    local box = row:CreateTexture(nil, "ARTWORK")
    box:SetSize(14, 14)
    box:SetPoint("TOPLEFT", 0, 1)
    row.box = box
    local text = row:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    text:SetPoint("TOPLEFT", box, "TOPRIGHT", 8, 0)
    text:SetPoint("TOPRIGHT", row, "TOPRIGHT", 0, 0)
    text:SetJustifyH("LEFT")
    text:SetJustifyV("TOP")
    text:SetWordWrap(true)
    text:SetSpacing(2)
    text:SetTextColor(INK.r, INK.g, INK.b)
    if text.SetHyperlinksEnabled then
        text:SetHyperlinksEnabled(true)
    end
    pcall(function()
        text:SetScript("OnHyperlinkClick", onGoalLinkClick)
        text:SetScript("OnHyperlinkEnter", onGoalLinkEnter)
        text:SetScript("OnHyperlinkLeave", onGoalLinkLeave)
    end)
    row:EnableMouse(true)
    bindWheel(row, detailScroll, 28)
    bindWheel(text, detailScroll, 28)
    row.text = text
    goalRows[i] = row
    return row
end

local function activityTooltipBody(level, race, className)
    local line = ""
    local n = tonumber(level) or 0
    if n > 0 then
        line = "Level " .. tostring(n)
    end
    race = plainString(race) or ""
    className = plainString(className) or ""
    if race ~= "" then
        if line ~= "" then
            line = line .. " "
        end
        line = line .. race
    end
    if className ~= "" then
        if line ~= "" then
            line = line .. " "
        end
        line = line .. className
    end
    return line
end

local function ensureActivityRow(i)
    local row = activityRows[i]
    if row then
        return row
    end
    row = CreateFrame("Frame", nil, detailChild)
    row:SetHeight(36)
    row:EnableMouse(true)
    local whenFS = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    whenFS:SetPoint("TOPLEFT", 0, 0)
    whenFS:SetJustifyH("LEFT")
    whenFS:SetJustifyV("TOP")
    whenFS:SetTextColor(INK_MUTED.r, INK_MUTED.g, INK_MUTED.b)
    local charFS = row:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    charFS:SetPoint("TOPLEFT", whenFS, "BOTTOMLEFT", 0, -2)
    charFS:SetJustifyH("LEFT")
    charFS:SetJustifyV("TOP")
    charFS:SetTextColor(INK.r, INK.g, INK.b)
    row.whenFS = whenFS
    row.charFS = charFS
    row:SetScript("OnEnter", function(self)
        if not GameTooltip then
            return
        end
        local name = self.tipName or ""
        if name == "" then
            return
        end
        GameTooltip:SetOwner(self, "ANCHOR_CURSOR")
        GameTooltip:ClearLines()
        GameTooltip:AddLine(name, 1, 1, 1)
        local tip = activityTooltipBody(self.tipLevel, self.tipRace, self.tipClass)
        if tip ~= "" then
            GameTooltip:AddLine(tip, 1, 1, 1)
        end
        local tag = plainString(self.tipTag)
        if tag then
            GameTooltip:AddLine(tag, 0.7, 0.7, 0.7)
        end
        GameTooltip:Show()
    end)
    row:SetScript("OnLeave", function()
        if GameTooltip then
            GameTooltip:Hide()
        end
    end)
    bindWheel(row, detailScroll, 28)
    activityRows[i] = row
    return row
end

local function ensureRow(i)
    local row = rows[i]
    if row then
        return row
    end
    row = CreateFrame("Button", nil, listChild)
    row:SetSize(210, 38)
    local bg = row:CreateTexture(nil, "BACKGROUND")
    bg:SetAllPoints()
    bg:SetTexture("Interface\\QuestFrame\\UI-QuestTitleHighlight")
    bg:SetBlendMode("ADD")
    bg:SetAlpha(0)
    row.bg = bg
    local hover = row:CreateTexture(nil, "BACKGROUND", nil, 1)
    hover:SetAllPoints()
    hover:SetTexture("Interface\\QuestFrame\\UI-QuestTitleHighlight")
    hover:SetBlendMode("ADD")
    hover:SetVertexColor(0.55, 0.42, 0.22)
    hover:SetAlpha(0)
    row.hover = hover
    local unread = row:CreateTexture(nil, "OVERLAY")
    unread:SetSize(9, 9)
    unread:SetPoint("TOPLEFT", 5, -6)
    paintUnreadDot(unread)
    unread:Hide()
    row.unread = unread
    local title = row:CreateFontString(nil, "OVERLAY", "GameFontNormal")
    title:SetPoint("TOPLEFT", LIST_TEXT_INSET, -4)
    title:SetPoint("TOPRIGHT", -8, -4)
    title:SetJustifyH("LEFT")
    title:SetWordWrap(false)
    row.title = title
    local sub = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    sub:SetPoint("BOTTOMLEFT", LIST_TEXT_INSET, 4)
    sub:SetPoint("BOTTOMRIGHT", -8, 4)
    sub:SetJustifyH("LEFT")
    row.sub = sub
    bindWheel(row, listScroll, 36)
    bindWheel(title, listScroll, 36)
    bindWheel(sub, listScroll, 36)
    row:SetScript("OnClick", function(self)
        selectedID = self.questID
        markRead(self.questID)
        refresh()
    end)
    row:SetScript("OnEnter", function(self)
        if self.questID ~= selectedID then
            self.hover:SetAlpha(0.45)
        end
    end)
    row:SetScript("OnLeave", function(self)
        self.hover:SetAlpha(0)
    end)
    rows[i] = row
    return row
end

local function placeSectionHeader(header, y, label, expanded, onToggle)
    header:ClearAllPoints()
    header:SetPoint("TOPLEFT", listChild, "TOPLEFT", 0, -y)
    header:SetPoint("TOPRIGHT", listChild, "TOPRIGHT", 0, -y)
    header:Show()
    if expanded then
        header.icon:SetTexture("Interface\\Buttons\\UI-MinusButton-UP")
    else
        header.icon:SetTexture("Interface\\Buttons\\UI-PlusButton-UP")
    end
    header.label:SetText(label)
    header:SetScript("OnClick", onToggle)
    return y + 24
end

refresh = function()
    if not built then
        return
    end
    local entries = currentEntries()

    local openList, doneList = {}, {}
    for i = 1, #entries do
        if isComplete(entries[i]) then
            table.insert(doneList, entries[i])
        else
            table.insert(openList, entries[i])
        end
    end

    local selectedEntry = selectedID and findEntry(selectedID) or nil
    if not selectedEntry then
        if openList[1] then
            selectedEntry = openList[1]
        elseif doneList[1] then
            selectedEntry = doneList[1]
        end
        if selectedEntry then
            selectedID = selectedEntry.quest_id
        end
    end

    local openExpanded = sectionExpanded("open", true)
    local doneExpanded = sectionExpanded("done", true)
    local listY = 0
    local rowIndex = 0

    listY = placeSectionHeader(sectionOpen, listY, "Ikke startet (" .. #openList .. ")", openExpanded, function()
        setSectionExpanded("open", not openExpanded)
        refresh()
    end)
    if openExpanded then
        for i = 1, #openList do
            rowIndex = rowIndex + 1
            local row = ensureRow(rowIndex)
            local item = openList[i]
            row:ClearAllPoints()
            row:SetPoint("TOPLEFT", listChild, "TOPLEFT", 4, -listY)
            row:Show()
            row.questID = item.quest_id
            row.title:SetText(item.title or "Saga")
            row.sub:SetText(listSubtitle(item, entries))
            layoutListRow(row, isUnread(item.quest_id, item))
            if entryLocked(item, entries) then
                row.title:SetTextColor(LOCKED_TITLE.r, LOCKED_TITLE.g, LOCKED_TITLE.b)
                row.sub:SetTextColor(LOCKED_SUB.r, LOCKED_SUB.g, LOCKED_SUB.b)
            else
                local c = difficultyColor(item.min_level, player.level)
                row.title:SetTextColor(c.r, c.g, c.b)
                row.sub:SetTextColor(0.72, 0.68, 0.58)
            end
            if item.quest_id == selectedID then
                row.bg:SetAlpha(0.7)
            else
                row.bg:SetAlpha(0)
            end
            listY = listY + 40
        end
    end

    listY = listY + 6
    listY = placeSectionHeader(sectionDone, listY, "Fullførte (" .. #doneList .. ")", doneExpanded, function()
        setSectionExpanded("done", not doneExpanded)
        refresh()
    end)
    if doneExpanded then
        for i = 1, #doneList do
            rowIndex = rowIndex + 1
            local row = ensureRow(rowIndex)
            local item = doneList[i]
            row:ClearAllPoints()
            row:SetPoint("TOPLEFT", listChild, "TOPLEFT", 4, -listY)
            row:Show()
            row.questID = item.quest_id
            row.title:SetText(item.title or "Saga")
            row.sub:SetText(creatorName(item))
            layoutListRow(row, false)
            row.title:SetTextColor(0.55, 0.75, 0.45)
            row.sub:SetTextColor(0.55, 0.52, 0.45)
            if item.quest_id == selectedID then
                row.bg:SetAlpha(0.55)
            else
                row.bg:SetAlpha(0)
            end
            listY = listY + 40
        end
    end

    for i = rowIndex + 1, #rows do
        rows[i]:Hide()
    end
    local listH = math.max(listY + 8, 10)
    listChild:SetHeight(listH)
    if listScroll.UpdateScrollChildRect then
        listScroll:UpdateScrollChildRect()
    end
    listScroll.range = math.max(0, listH - 418)

    if selectedID ~= lastDetailID then
        lastDetailID = selectedID
        detailScroll.offset = 0
        detailScroll:SetVerticalScroll(0)
    end
    local width = 348
    detailChild:SetWidth(width + 12)
    local detailY = 4
    local goals = {}
    detailFS:Show()
    if not selectedEntry then
        detailTitle:SetText("")
        detailTitle:Hide()
        detailGoalsHeader:Hide()
        if detailImage then
            detailImage:Hide()
        end
        if detailActivityHeader then
            detailActivityHeader:Hide()
        end
        if detailMeta then
            detailMeta:Hide()
        end
        if player.name == "" then
            detailFS:SetText("Logg inn på en karakter.")
        elseif type(GutSagaData) ~= "table" or type(GutSagaData.entries) ~= "table" or #GutSagaData.entries == 0 then
            detailFS:SetText("Ingen sagaer ennå.\n\nStart GutSaga på maskinen og skriv /reload.")
        else
            detailFS:SetText("Ingen sagaer for " .. player.name .. ".")
        end
        detailFS:SetTextColor(INK.r, INK.g, INK.b)
        detailFS:SetWidth(width)
        detailFS:ClearAllPoints()
        detailFS:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailY = detailY + (detailFS:GetStringHeight() or 14) + 8
    else
        local selectedLocked = entryLocked(selectedEntry, entries)
        detailTitle:Show()
        detailTitle:SetText(selectedEntry.title or "Saga")
        -- Parchment header stays dark ink for contrast; list titles carry difficulty color.
        detailTitle:SetTextColor(INK.r, INK.g, INK.b)
        detailTitle:SetWidth(width)
        detailTitle:ClearAllPoints()
        detailTitle:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailY = detailY + (detailTitle:GetStringHeight() or 20) + 8

        local meta = ""
        if type(GutSagaChains) == "table" and type(GutSagaChains.detailLines) == "function" then
            meta = GutSagaChains.detailLines(selectedEntry, entries, completionSet()) or ""
        end
        if detailMeta then
            if meta ~= "" then
                detailMeta:Show()
                detailMeta:SetText(meta)
                detailMeta:SetWidth(width)
                detailMeta:ClearAllPoints()
                detailMeta:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
                if selectedLocked then
                    detailMeta:SetTextColor(0.45, 0.28, 0.18)
                else
                    detailMeta:SetTextColor(INK_MUTED.r, INK_MUTED.g, INK_MUTED.b)
                end
                detailY = detailY + (detailMeta:GetStringHeight() or 14) + 10
            else
                detailMeta:Hide()
            end
        end

        local okGoals, goalList = pcall(objectiveLines, selectedEntry)
        if okGoals and type(goalList) == "table" then
            goals = goalList
        end
    end

    local selectedLocked = selectedEntry and entryLocked(selectedEntry, entries)
    -- Goals sit under the title with no section header.
    for i, goal in ipairs(goals) do
        local row = ensureGoalRow(i)
        local indent = tonumber(goal.indent) or 0
        if indent < 0 then
            indent = 0
        elseif indent > 3 then
            indent = 3
        end
        local left = 6 + indent * 16
        local rowWidth = width - indent * 16
        local textWidth = rowWidth - 22
        row:ClearAllPoints()
        row:SetPoint("TOPLEFT", detailChild, "TOPLEFT", left, -detailY)
        row:SetWidth(rowWidth)
        row:Show()
        if goal.done then
            row.box:SetTexture("Interface\\AddOns\\GutSaga\\check")
            row.box:SetVertexColor(INK.r, INK.g, INK.b)
            row.text:SetTextColor(INK_MUTED.r, INK_MUTED.g, INK_MUTED.b)
        elseif goal.failed then
            row.box:SetTexture("Interface\\AddOns\\GutSaga\\box")
            row.box:SetVertexColor(0.45, 0.42, 0.38)
            row.text:SetTextColor(0.45, 0.42, 0.38)
        elseif selectedLocked then
            row.box:SetTexture("Interface\\AddOns\\GutSaga\\box")
            row.box:SetVertexColor(0.55, 0.55, 0.55)
            row.text:SetTextColor(INK_MUTED.r, INK_MUTED.g, INK_MUTED.b)
        else
            row.box:SetTexture("Interface\\AddOns\\GutSaga\\box")
            row.box:SetVertexColor(1, 1, 1)
            row.text:SetTextColor(INK.r, INK.g, INK.b)
        end
        row.text:SetWidth(textWidth)
        row.text:SetText(goal.text or "")
        local h = math.max(16, (row.text:GetStringHeight() or 14) + 4)
        row:SetHeight(h)
        detailY = detailY + h + 10
    end
    for i = #goals + 1, #goalRows do
        goalRows[i]:Hide()
    end

    if selectedEntry then
        if #goals > 0 then
            detailY = detailY + 4
        end
        detailGoalsHeader:Show()
        detailGoalsHeader:ClearAllPoints()
        detailGoalsHeader:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailGoalsHeader:SetWidth(width)
        detailGoalsHeader:SetTextColor(INK.r, INK.g, INK.b)
        detailY = detailY + (detailGoalsHeader:GetStringHeight() or 20) + 10

        local description = formatDescription(selectedEntry.description)
        detailFS:SetText(description)
        detailFS:SetTextColor(INK.r, INK.g, INK.b)
        detailFS:SetSpacing(4)
        detailFS:SetWidth(width)
        detailFS:ClearAllPoints()
        detailFS:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailY = detailY + (detailFS:GetStringHeight() or 14) + 12

        if detailImage then
            local path = selectedEntry.image
            if type(path) == "string" and path ~= "" then
                detailImage:ClearAllPoints()
                detailImage:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
                local aspect = tonumber(selectedEntry.image_aspect) or 1
                if aspect < 0.2 then
                    aspect = 1
                elseif aspect > 5 then
                    aspect = 5
                end
                local imgW = width
                local imgH = math.floor(imgW / aspect + 0.5)
                if imgH > 260 then
                    imgH = 260
                    imgW = math.floor(imgH * aspect + 0.5)
                end
                if imgH < 64 then
                    imgH = 64
                end
                detailImage:SetSize(imgW, imgH)
                detailImage:SetVertexColor(1, 1, 1, 1)
                -- Classic loads AddOn textures as .tga; path is without extension.
                detailImage:SetTexture(path)
                detailImage:Show()
                detailY = detailY + imgH + 14
            else
                detailImage:Hide()
            end
        end
    end

    local activities = {}
    if selectedEntry then
        local okAct, actList = pcall(activityList, selectedEntry)
        if okAct and type(actList) == "table" then
            activities = actList
        end
    end
    if detailActivityHeader then
        if #activities > 0 then
            detailY = detailY + 8
            detailActivityHeader:Show()
            detailActivityHeader:ClearAllPoints()
            detailActivityHeader:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
            detailActivityHeader:SetWidth(width)
            detailActivityHeader:SetTextColor(INK.r, INK.g, INK.b)
            detailY = detailY + (detailActivityHeader:GetStringHeight() or 20) + 10
        else
            detailActivityHeader:Hide()
        end
    end
    for i, act in ipairs(activities) do
        local okRow, errRow = pcall(function()
            local row = ensureActivityRow(i)
            row:ClearAllPoints()
            row:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
            row:SetWidth(width)
            row:Show()
            if row.userFS then
                row.userFS:Hide()
            end
            local whenText = act.when or ""
            local charText = act.character or ""
            local line = charText
            if line ~= "" then
                line = line .. " fullførte"
            end
            row.whenFS:SetWidth(width)
            row.charFS:SetWidth(width)
            row.whenFS:SetText(whenText)
            row.charFS:SetText(line)
            row.charFS:SetTextColor(INK.r, INK.g, INK.b)
            row.whenFS:SetTextColor(INK_MUTED.r, INK_MUTED.g, INK_MUTED.b)
            row.tipName = charText
            row.tipLevel = act.level or 0
            row.tipRace = act.race or ""
            row.tipClass = act.class or ""
            row.tipTag = act.battletag or ""
            local h = (row.whenFS:GetStringHeight() or 12) + 2
                + (row.charFS:GetStringHeight() or 14)
            h = math.max(28, h + 4)
            row:SetHeight(h)
            detailY = detailY + h + 10
        end)
        if not okRow then
            chat("[GutSaga] aktivitet: " .. tostring(errRow))
        end
    end
    for i = #activities + 1, #activityRows do
        activityRows[i]:Hide()
    end

    local detailH = math.max(detailY + 8, 10)
    detailChild:SetHeight(detailH)
    if detailScroll.UpdateScrollChildRect then
        detailScroll:UpdateScrollChildRect()
    end
    detailScroll.range = math.max(0, detailH - 410)
    pcall(syncScrollBar, listScroll)
    pcall(syncScrollBar, detailScroll)
    updateMinimapUnread()
end

local function encodeMembers(members)
    local parts = {}
    if type(members) ~= "table" then
        return ""
    end
    for i = 1, #members do
        local m = members[i]
        if type(m) == "table" and type(m.name) == "string" and m.name ~= "" then
            local name = (m.name:gsub("[/;]", ""))
            local realm = ((m.realm or ""):gsub("[/;]", ""))
            table.insert(parts, string.format("%s/%s/%d", name, realm, tonumber(m.level) or 0))
        end
    end
    return table.concat(parts, ";")
end

local function placeLabel(rule, group)
    group = type(group) == "table" and group or {}
    local inst = group.instance or ""
    local zone = group.zone or ""
    if type(rule) ~= "table" or type(rule.instance) ~= "string" or rule.instance == "" then
        if inst ~= "" then
            return inst
        end
        return zone
    end
    if inst ~= "" and GutSagaObjectives and GutSagaObjectives.groupOK then
        local probe = { instance = inst, zone = "", members = { { name = player.name ~= "" and player.name or "x", level = 100 } } }
        if GutSagaObjectives.groupOK({ size = 1, instance = rule.instance }, probe) then
            return inst
        end
    end
    if zone ~= "" then
        return zone
    end
    return inst
end

local function rememberAttest(questID, objID, name, at)
    if type(questID) ~= "string" or questID == "" or type(objID) ~= "string" or objID == "" then
        return
    end
    local who = norm(name)
    if who == "" or not at or at <= 0 then
        return
    end
    if type(world.attests) ~= "table" then
        world.attests = {}
    end
    if type(world.attests[questID]) ~= "table" then
        world.attests[questID] = {}
    end
    if type(world.attests[questID][objID]) ~= "table" then
        world.attests[questID][objID] = {}
    end
    world.attests[questID][objID][who] = at
end

local function sendHerePing(questID, objID)
    if not ((C_ChatInfo and C_ChatInfo.SendAddonMessage) or SendAddonMessage) then
        return
    end
    local raid, grouped = false, false
    pcall(function()
        raid = IsInRaid() and true or false
    end)
    pcall(function()
        grouped = IsInGroup() and true or false
    end)
    local channel
    if raid then
        channel = "RAID"
    elseif grouped then
        channel = "PARTY"
    else
        return
    end
    sendAddon(PREFIX, "H\t" .. tostring(questID) .. "\t" .. tostring(objID), channel)
end

-- A group kill only counts when this client is in the instance at the exact
-- level. The ping lets every other addon attest for itself. Someone outside
-- the dungeon never sends one.
local function witnessGroupKills(deathName)
    if type(deathName) ~= "string" or deathName == "" or not GutSagaObjectives or not GutSagaObjectives.listed then
        return
    end
    cachePlayer()
    if player.name == "" then
        return
    end
    local group = readGroup()
    local now = unixNow()
    for _, entry in ipairs(currentEntries()) do
        if type(entry) == "table" and not isComplete(entry) and type(entry.objectives) == "table" then
            for _, obj in ipairs(entry.objectives) do
                if type(obj) == "table" and obj.type == "kill" and type(obj.group) == "table" and tonumber(obj.group.level) then
                    local level = tonumber(obj.group.level) or 0
                    local inst = obj.group.instance
                    local here = type(inst) ~= "string" or inst == ""
                        or GutSagaObjectives.listed(inst, group.instance)
                        or GutSagaObjectives.listed(inst, group.zone)
                    if here and (tonumber(player.level) or 0) == level and GutSagaObjectives.listed(obj.npc, deathName) then
                        rememberAttest(entry.quest_id, obj.id, player.name, now)
                        sendHerePing(entry.quest_id, obj.id)
                    end
                end
            end
        end
    end
end

local function sendGroupPing(questID, version)
    if not ((C_ChatInfo and C_ChatInfo.SendAddonMessage) or SendAddonMessage) then
        return
    end
    local raid, grouped = false, false
    pcall(function()
        raid = IsInRaid() and true or false
    end)
    pcall(function()
        grouped = IsInGroup() and true or false
    end)
    local channel
    if raid then
        channel = "RAID"
    elseif grouped then
        channel = "PARTY"
    else
        return
    end
    -- Party/raid only, including inside instances. Guild toasts stay outside.
    sendAddon(PREFIX, "G\t" .. tostring(questID) .. "\t" .. tostring(version), channel)
end

finishQuest = function(entry, group)
    if not entry or isComplete(entry) then
        return
    end
    local entries = currentEntries()
    if entryLocked(entry, entries) then
        return
    end
    cachePlayer()
    initDB()
    if player.name == "" then
        return
    end
    local before = completionSet()
    local version = tonumber(entry.quest_version) or 0
    local level = player.level
    if level < 1 then
        level = 1
    end
    local ts = unixNow()
    local row = {
        quest_id = entry.quest_id,
        version = version,
        level = level,
        completed_at = ts,
        character = player.name,
        realm = player.realm,
        race = player.race,
        class = player.class,
    }
    if type(entry.group) == "table" and type(group) == "table" then
        row.group_members = encodeMembers(group.members)
        row.instance = placeLabel(entry.group, group)
    end
    GutSagaDB.completions[entry.quest_id] = row
    if type(entry.group) == "table" then
        sendGroupPing(entry.quest_id, version)
    end
    local title = entry.title or "Saga"
    local questID = entry.quest_id
    local unlocked = {}
    if type(GutSagaChains) == "table" and type(GutSagaChains.newlyUnlocked) == "function" then
        unlocked = GutSagaChains.newlyUnlocked(entries, before, completionSet()) or {}
    end
    if unlocked[1] and type(unlocked[1].quest_id) == "string" then
        selectedID = unlocked[1].quest_id
    end
    showSagaToast(title, questID)
    broadcast(string.format("C|%s|%d|%d|%d", questID, version, level, ts))
    refresh()
    if DEFAULT_CHAT_FRAME and DEFAULT_CHAT_FRAME.AddMessage then
        DEFAULT_CHAT_FRAME:AddMessage(
            "|cffc9a227GutSaga|r: Fullført. Discord etter |cffffffff/reload|r."
        )
        local n = #unlocked
        if n > 3 then
            n = 3
        end
        for i = 1, n do
            DEFAULT_CHAT_FRAME:AddMessage(
                "|cffc9a227GutSaga|r: Ny saga: " .. (unlocked[i].title or "Saga")
            )
        end
    end
end

local function onWorld()
    readLocation()
    readNPC()
    readObject()
    checkAll()
    if frame and frame:IsShown() then
        refresh()
    else
        updateMinimapUnread()
    end
end

local function show()
    if frame then
        frame:Show()
        onWorld()
        refresh()
    end
end

local function hide()
    if frame then
        frame:Hide()
    end
end

local function toggle()
    if frame and frame:IsShown() then
        hide()
    else
        show()
    end
end

local function wheelDelta(delta)
    local n = plainNumber(delta)
    if n then
        if n > 0 then
            return 1
        end
        if n < 0 then
            return -1
        end
        return 0
    end
    -- Secret/tainted deltas: never branch on the raw value; only recover a plain sign.
    local ok, sign = pcall(function()
        if delta > 0 then
            return 1
        end
        if delta < 0 then
            return -1
        end
        return 0
    end)
    if ok then
        return plainNumber(sign) or 1
    end
    return 1
end

syncScrollBar = function(scroll)
    local bar = scroll and scroll.bar
    if not bar then
        return
    end
    if scroll.UpdateScrollChildRect then
        scroll:UpdateScrollChildRect()
    end
    local range = scroll.range or 0
    if scroll.GetVerticalScrollRange then
        local nativeRange = scroll:GetVerticalScrollRange() or 0
        if nativeRange > range then
            range = nativeRange
        end
    end
    if range < 0 then
        range = 0
    end
    scroll.range = range
    local cur = scroll.offset or 0
    if cur < 0 then
        cur = 0
    elseif cur > range then
        cur = range
    end
    scroll.offset = cur
    scroll:SetVerticalScroll(cur)
    local maxVal = math.max(1, range)
    bar.skip = true
    bar:SetMinMaxValues(0, maxVal)
    -- Force WoW's Slider to place the thumb texture even when cur is 0 on first open.
    bar:SetValue(cur == 0 and maxVal or 0)
    bar:SetValue(cur)
    bar.skip = false
    if bar.thumb then
        if range > 0 then
            bar.thumb:Show()
        else
            bar.thumb:Hide()
        end
    end
end

scrollBy = function(self, delta, step)
    if not self then
        return
    end
    local dir = wheelDelta(delta)
    if dir == 0 then
        return
    end
    if self.UpdateScrollChildRect then
        self:UpdateScrollChildRect()
    end
    local range = self.range or 0
    if self.GetVerticalScrollRange then
        local nativeRange = self:GetVerticalScrollRange() or 0
        if nativeRange > range then
            range = nativeRange
        end
    end
    if range <= 0 then
        return
    end
    local nxt = (self.offset or 0) - dir * (step or 40)
    if nxt < 0 then
        nxt = 0
    elseif nxt > range then
        nxt = range
    end
    self.offset = nxt
    -- Drive the slider when present so thumb + OnValueChanged stay aligned.
    local bar = self.bar
    if bar then
        bar.skip = true
        bar:SetMinMaxValues(0, math.max(1, range))
        bar:SetValue(nxt == 0 and math.max(1, range) or 0)
        bar:SetValue(nxt)
        bar.skip = false
    end
    self:SetVerticalScroll(nxt)
    syncScrollBar(self)
end

bindWheel = function(target, scroll, step)
    if not target then
        return
    end
    pcall(function()
        if target.EnableMouseWheel then
            target:EnableMouseWheel(true)
        end
        target:SetScript("OnMouseWheel", function(_, delta)
            scrollBy(scroll, delta, step)
        end)
    end)
end

wheelOnList = function(_, delta)
    scrollBy(listScroll, delta, 36)
end

wheelOnDetail = function(_, delta)
    scrollBy(detailScroll, delta, 28)
end

wheelOnFrame = function(_, delta)
    local scale = frame and frame:GetEffectiveScale() or 1
    if scale == 0 then
        scale = 1
    end
    local cx = GetCursorPosition()
    cx = (plainNumber(cx) or 0) / scale
    local left = detailPanel and detailPanel:GetLeft()
    left = plainNumber(left)
    if left and cx >= left then
        scrollBy(detailScroll, delta, 28)
    else
        scrollBy(listScroll, delta, 36)
    end
end

local function makeScrollArrow(parent, up)
    local btn = CreateFrame("Button", nil, parent)
    btn:SetSize(18, 16)
    local tex = btn:CreateTexture(nil, "ARTWORK")
    tex:SetPoint("TOPLEFT", -4, 4)
    tex:SetPoint("BOTTOMRIGHT", 4, -4)
    if up then
        tex:SetTexture("Interface\\Buttons\\UI-ScrollBar-ScrollUpButton-Up")
    else
        tex:SetTexture("Interface\\Buttons\\UI-ScrollBar-ScrollDownButton-Up")
    end
    btn.tex = tex
    return btn
end

local function attachScrollBar(parent, scroll, step)
    local up = makeScrollArrow(parent, true)
    local down = makeScrollArrow(parent, false)
    up:SetPoint("TOPRIGHT", parent, "TOPRIGHT", -5, -6)
    down:SetPoint("BOTTOMRIGHT", parent, "BOTTOMRIGHT", -5, 6)
    local bar = CreateFrame("Slider", nil, parent)
    bar:SetOrientation("VERTICAL")
    bar:SetWidth(16)
    bar:SetPoint("TOP", up, "BOTTOM", 0, -2)
    bar:SetPoint("BOTTOM", down, "TOP", 0, 2)
    bar:SetMinMaxValues(0, 1)
    bar:SetValueStep(1)
    local trackBorder = bar:CreateTexture(nil, "BACKGROUND")
    trackBorder:SetPoint("TOPLEFT", 2, 2)
    trackBorder:SetPoint("BOTTOMRIGHT", -2, -2)
    trackBorder:SetColorTexture(0.38, 0.28, 0.14, 0.9)
    local track = bar:CreateTexture(nil, "BORDER")
    track:SetPoint("TOPLEFT", 3, 1)
    track:SetPoint("BOTTOMRIGHT", -3, -1)
    track:SetColorTexture(0.08, 0.05, 0.03, 0.95)
    bar:SetThumbTexture("Interface\\Buttons\\UI-ScrollBar-Knob")
    local thumb = bar:GetThumbTexture()
    if thumb then
        thumb:SetSize(24, 24)
    end
    bar.thumb = thumb
    bar:EnableMouse(true)
    bar:EnableMouseWheel(true)
    bar:SetScript("OnValueChanged", function(self, value)
        if self.skip then
            return
        end
        local v = value or 0
        scroll.offset = v
        if scroll.UpdateScrollChildRect then
            scroll:UpdateScrollChildRect()
        end
        scroll:SetVerticalScroll(v)
    end)
    scroll:SetScript("OnScrollRangeChanged", function(self)
        syncScrollBar(self)
    end)
    bar:SetScript("OnMouseWheel", function(_, delta)
        scrollBy(scroll, delta, step)
    end)
    up:EnableMouseWheel(true)
    up:SetScript("OnClick", function()
        scrollBy(scroll, 1, step)
    end)
    up:SetScript("OnMouseWheel", function(_, delta)
        scrollBy(scroll, delta, step)
    end)
    down:EnableMouseWheel(true)
    down:SetScript("OnClick", function()
        scrollBy(scroll, -1, step)
    end)
    down:SetScript("OnMouseWheel", function(_, delta)
        scrollBy(scroll, delta, step)
    end)
    bar.up = up
    bar.down = down
    scroll.bar = bar
    return bar
end

local function useBackdrop(f, info, border)
    if BackdropTemplateMixin and not f.SetBackdrop then
        Mixin(f, BackdropTemplateMixin)
    end
    if f.SetBackdrop then
        f:SetBackdrop(info)
    end
    if border and f.SetBackdropBorderColor then
        f:SetBackdropBorderColor(border.r, border.g, border.b, 1)
    end
end

local function makeSectionHeader(parent)
    local btn = CreateFrame("Button", nil, parent)
    btn:SetHeight(22)
    local bg = btn:CreateTexture(nil, "BACKGROUND")
    bg:SetAllPoints()
    bg:SetColorTexture(0.18, 0.12, 0.05, 0.85)
    btn.bg = bg
    local icon = btn:CreateTexture(nil, "ARTWORK")
    icon:SetSize(16, 16)
    icon:SetPoint("LEFT", 4, 0)
    icon:SetTexture("Interface\\Buttons\\UI-MinusButton-UP")
    btn.icon = icon
    local highlight = btn:CreateTexture(nil, "HIGHLIGHT")
    highlight:SetAllPoints(icon)
    highlight:SetTexture("Interface\\Buttons\\UI-PlusButton-Hilight")
    highlight:SetBlendMode("ADD")
    local label = btn:CreateFontString(nil, "OVERLAY", "GameFontNormal")
    label:SetPoint("LEFT", icon, "RIGHT", 4, 0)
    label:SetTextColor(GOLD.r, GOLD.g, GOLD.b)
    btn.label = label
    bindWheel(btn, listScroll, 36)
    return btn
end

local function build()
    if built then
        return
    end
    built = true
    local backdropTemplate = BackdropTemplateMixin and "BackdropTemplate" or nil
    frame = CreateFrame("Frame", "GutSagaFrame", UIParent, backdropTemplate)
    frame:SetSize(700, 500)
    frame:SetPoint("CENTER")
    frame:SetFrameStrata("HIGH")
    frame:SetMovable(true)
    frame:SetClampedToScreen(true)
    frame:EnableMouse(true)
    frame:EnableMouseWheel(true)
    frame:SetScript("OnMouseWheel", wheelOnFrame)
    frame:Hide()
    table.insert(UISpecialFrames, "GutSagaFrame")
    useBackdrop(frame, {
        bgFile = "Interface\\DialogFrame\\UI-DialogBox-Background",
        edgeFile = "Interface\\DialogFrame\\UI-DialogBox-Border",
        tile = true,
        tileSize = 32,
        edgeSize = 32,
        insets = { left = 11, right = 12, top = 12, bottom = 11 },
    }, GOLD)

    local header = frame:CreateTexture(nil, "ARTWORK")
    header:SetTexture("Interface\\DialogFrame\\UI-DialogBox-Header")
    header:SetSize(300, 64)
    header:SetPoint("TOP", frame, "TOP", 0, 12)
    header:SetVertexColor(GOLD.r, GOLD.g, GOLD.b)
    local title = frame:CreateFontString(nil, "OVERLAY", "GameFontNormal")
    title:SetPoint("TOP", header, "TOP", 0, -14)
    title:SetText("GutSaga")
    title:SetTextColor(1, 0.95, 0.72)

    local mark = frame:CreateTexture(nil, "OVERLAY")
    mark:SetSize(58, 58)
    mark:SetPoint("TOPLEFT", frame, "TOPLEFT", -10, 14)
    mark:SetTexture("Interface\\AddOns\\GutSaga\\GutSaga")

    local close = CreateFrame("Button", nil, frame, "UIPanelCloseButton")
    close:SetPoint("TOPRIGHT", frame, "TOPRIGHT", -2, -2)

    local drag = CreateFrame("Frame", nil, frame)
    drag:SetPoint("TOPLEFT", frame, "TOPLEFT", 56, 10)
    drag:SetPoint("TOPRIGHT", frame, "TOPRIGHT", -46, 10)
    drag:SetHeight(28)
    drag:EnableMouse(true)
    drag:EnableMouseWheel(true)
    drag:SetScript("OnMouseWheel", wheelOnFrame)
    drag:RegisterForDrag("LeftButton")
    drag:SetScript("OnDragStart", function()
        frame:StartMoving()
    end)
    drag:SetScript("OnDragStop", function()
        frame:StopMovingOrSizing()
        local point, _, rel, x, y = frame:GetPoint()
        if type(GutSagaDB) == "table" and type(point) == "string" and type(rel) == "string" then
            GutSagaDB.point = { point, rel, x, y }
        end
    end)

    if type(GutSagaDB) == "table" and type(GutSagaDB.point) == "table" then
        frame:ClearAllPoints()
        frame:SetPoint(GutSagaDB.point[1], UIParent, GutSagaDB.point[2], GutSagaDB.point[3], GutSagaDB.point[4])
    end

    listPanel = CreateFrame("Frame", nil, frame, backdropTemplate)
    listPanel:SetPoint("TOPLEFT", 22, -48)
    listPanel:SetPoint("BOTTOMLEFT", 22, 18)
    listPanel:SetSize(236, 434)
    useBackdrop(listPanel, {
        bgFile = "Interface\\Buttons\\WHITE8X8",
        edgeFile = "Interface\\Tooltips\\UI-Tooltip-Border",
        tile = true,
        tileSize = 8,
        edgeSize = 12,
        insets = { left = 3, right = 3, top = 3, bottom = 3 },
    }, GOLD)
    if listPanel.SetBackdropColor then
        listPanel:SetBackdropColor(0.07, 0.05, 0.03, 0.92)
    end

    listScroll = CreateFrame("ScrollFrame", "GutSagaListScroll", listPanel)
    listScroll:SetPoint("TOPLEFT", 6, -8)
    listScroll:SetPoint("BOTTOMRIGHT", -28, 8)
    listScroll:SetSize(202, 418)
    listChild = CreateFrame("Frame", "GutSagaListChild", listScroll)
    listChild:SetSize(196, 418)
    listScroll:SetScrollChild(listChild)
    listPanel:EnableMouse(true)
    listScroll:EnableMouse(true)
    listChild:EnableMouse(true)
    bindWheel(listPanel, listScroll, 36)
    bindWheel(listScroll, listScroll, 36)
    bindWheel(listChild, listScroll, 36)
    sectionOpen = makeSectionHeader(listChild)
    sectionDone = makeSectionHeader(listChild)
    attachScrollBar(listPanel, listScroll, 36)

    -- Same top and bottom as the sidebar. Explicit size keeps ScrollFrame
    -- clipping valid even when refreshed before the window is first shown.
    detailPanel = CreateFrame("Frame", nil, frame, backdropTemplate)
    detailPanel:SetPoint("TOPLEFT", listPanel, "TOPRIGHT", 14, 0)
    detailPanel:SetPoint("BOTTOMRIGHT", frame, "BOTTOMRIGHT", -18, 18)
    detailPanel:SetSize(410, 434)
    useBackdrop(detailPanel, {
        bgFile = "Interface\\Buttons\\WHITE8X8",
        edgeFile = "Interface\\Tooltips\\UI-Tooltip-Border",
        tile = true,
        tileSize = 8,
        edgeSize = 14,
        insets = { left = 4, right = 4, top = 4, bottom = 4 },
    }, GOLD)
    if detailPanel.SetBackdropColor then
        detailPanel:SetBackdropColor(0.86, 0.78, 0.58, 0.97)
    end
    local parchment = detailPanel:CreateTexture(nil, "BACKGROUND", nil, 1)
    parchment:SetPoint("TOPLEFT", 5, -5)
    parchment:SetPoint("BOTTOMRIGHT", -5, 5)
    parchment:SetTexture("Interface\\QuestFrame\\QuestBG")
    parchment:SetTexCoord(0, 0.58, 0.02, 0.66)
    parchment:SetVertexColor(1, 0.97, 0.9)

    detailScroll = CreateFrame("ScrollFrame", "GutSagaDetailScroll", detailPanel)
    detailScroll:SetPoint("TOPLEFT", 12, -12)
    detailScroll:SetPoint("BOTTOMRIGHT", -30, 12)
    detailScroll:SetSize(368, 410)
    detailChild = CreateFrame("Frame", "GutSagaDetailChild", detailScroll)
    detailChild:SetSize(360, 410)
    detailScroll:SetScrollChild(detailChild)
    detailPanel:EnableMouse(true)
    detailScroll:EnableMouse(true)
    detailChild:EnableMouse(true)
    bindWheel(detailPanel, detailScroll, 28)
    bindWheel(detailScroll, detailScroll, 28)
    bindWheel(detailChild, detailScroll, 28)
    local titleFont = "GameFontNormalLarge"
    if QuestTitleFont then
        titleFont = "QuestTitleFont"
    end
    detailTitle = detailChild:CreateFontString(nil, "OVERLAY", titleFont)
    detailTitle:SetPoint("TOPLEFT", 6, -4)
    detailTitle:SetJustifyH("LEFT")
    detailTitle:SetJustifyV("TOP")
    detailTitle:SetWordWrap(true)
    detailTitle:SetTextColor(INK.r, INK.g, INK.b)
    detailMeta = detailChild:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    detailMeta:SetJustifyH("LEFT")
    detailMeta:SetJustifyV("TOP")
    detailMeta:SetWordWrap(true)
    detailMeta:SetSpacing(2)
    detailMeta:SetTextColor(INK_MUTED.r, INK_MUTED.g, INK_MUTED.b)
    detailMeta:Hide()
    detailFS = detailChild:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    detailFS:SetPoint("TOPLEFT", detailTitle, "BOTTOMLEFT", 0, -10)
    detailFS:SetJustifyH("LEFT")
    detailFS:SetJustifyV("TOP")
    detailFS:SetWordWrap(true)
    detailFS:SetSpacing(2)
    detailFS:SetTextColor(INK.r, INK.g, INK.b)
    if detailFS.SetHyperlinksEnabled then
        detailFS:SetHyperlinksEnabled(true)
    end
    detailImage = detailChild:CreateTexture(nil, "ARTWORK")
    detailImage:SetTexCoord(0, 1, 0, 1)
    detailImage:SetVertexColor(1, 1, 1, 1)
    detailImage:Hide()
    detailGoalsHeader = detailChild:CreateFontString(nil, "OVERLAY", titleFont)
    detailGoalsHeader:SetJustifyH("LEFT")
    detailGoalsHeader:SetText("Beskrivelse")
    detailGoalsHeader:SetTextColor(INK.r, INK.g, INK.b)
    detailActivityHeader = detailChild:CreateFontString(nil, "OVERLAY", titleFont)
    detailActivityHeader:SetJustifyH("LEFT")
    detailActivityHeader:SetText("Aktivitet")
    detailActivityHeader:SetTextColor(INK.r, INK.g, INK.b)
    detailActivityHeader:Hide()
    pcall(function()
        detailFS:SetScript("OnHyperlinkClick", onGoalLinkClick)
        detailFS:SetScript("OnHyperlinkEnter", onGoalLinkEnter)
        detailFS:SetScript("OnHyperlinkLeave", onGoalLinkLeave)
    end)
    bindWheel(detailTitle, detailScroll, 28)
    bindWheel(detailMeta, detailScroll, 28)
    bindWheel(detailFS, detailScroll, 28)
    bindWheel(detailGoalsHeader, detailScroll, 28)
    bindWheel(detailActivityHeader, detailScroll, 28)
    attachScrollBar(detailPanel, detailScroll, 28)

    toast = CreateFrame("Frame", "GutSagaToast", UIParent, backdropTemplate)
    toast:SetSize(390, 92)
    toast:SetFrameStrata("DIALOG")
    toast:SetPoint("TOP", UIParent, "TOP", 0, -30)
    toast:Hide()
    useBackdrop(toast, {
        bgFile = "Interface\\DialogFrame\\UI-DialogBox-Background",
        edgeFile = "Interface\\DialogFrame\\UI-DialogBox-Border",
        tile = true,
        tileSize = 32,
        edgeSize = 24,
        insets = { left = 6, right = 6, top = 6, bottom = 6 },
    }, GOLD)
    local toastIcon = toast:CreateTexture(nil, "ARTWORK")
    toastIcon:SetSize(58, 58)
    toastIcon:SetPoint("LEFT", 18, 0)
    toastIcon:SetTexture("Interface\\AddOns\\GutSaga\\GutSaga")
    local toastHeading = toast:CreateFontString(nil, "OVERLAY", "GameFontNormal")
    toastHeading:SetPoint("TOPLEFT", toastIcon, "TOPRIGHT", 14, -8)
    toastHeading:SetText("SAGA FULLFØRT")
    local toastTitle = toast:CreateFontString(nil, "OVERLAY", "GameFontNormalLarge")
    toastTitle:SetPoint("TOPLEFT", toastHeading, "BOTTOMLEFT", 0, -4)
    toastTitle:SetPoint("RIGHT", toast, "RIGHT", -18, 0)
    toastTitle:SetJustifyH("LEFT")
    toastTitle:SetWordWrap(false)
    toastTitle:SetTextColor(1, 0.95, 0.8)
    -- Ease in (~0.35s), hold longer, then a simple fade (no slide-out).
    local TOAST_EASE = 0.35
    local TOAST_HOLD = 7.0
    local TOAST_FADE = 0.55
    toast:SetScript("OnUpdate", function(self, elapsed)
        if not self:IsShown() then
            return
        end
        self.age = (self.age or 0) + (elapsed or 0)
        local age = self.age
        local alpha, y = 1, -30
        if age < TOAST_EASE then
            local t = age / TOAST_EASE
            alpha = t
            y = 16 - 46 * t
        elseif age < TOAST_HOLD then
            alpha = 1
            y = -30
        elseif age < TOAST_HOLD + TOAST_FADE then
            local t = (age - TOAST_HOLD) / TOAST_FADE
            alpha = 1 - t
            y = -30
        else
            self:Hide()
            return
        end
        self:SetAlpha(alpha)
        self:ClearAllPoints()
        self:SetPoint("TOP", UIParent, "TOP", 0, y)
    end)
    toast:EnableMouse(true)
    toast:SetScript("OnMouseUp", function(self, button)
        if button ~= "LeftButton" then
            return
        end
        local questID = self.questID
        self:Hide()
        if type(questID) == "string" and questID ~= "" then
            selectedID = questID
            markRead(questID)
            show()
            refresh()
        end
    end)
    toast:SetScript("OnEnter", function(self)
        if GameTooltip then
            GameTooltip:SetOwner(self, "ANCHOR_BOTTOM")
            GameTooltip:SetText("Åpne saga-loggen", 1, 1, 1)
            GameTooltip:Show()
        end
    end)
    toast:SetScript("OnLeave", function()
        if GameTooltip then
            GameTooltip:Hide()
        end
    end)
    showSagaToast = function(titleText, questID)
        toastTitle:SetText(titleText or "Saga")
        toast.questID = questID
        toast.age = 0
        toast:SetAlpha(0)
        toast:Show()
        if PlaySoundFile then
            pcall(PlaySoundFile, "Interface\\AddOns\\GutSaga\\complete.mp3", "Master")
        end
    end

    if Minimap then
        local btn = CreateFrame("Button", "GutSagaMinimapButton", Minimap)
        minimapBtn = btn
        -- Larger hit/visual size so the emblem is not crushed into ~20px.
        btn:SetSize(40, 40)
        btn:SetFrameStrata("MEDIUM")
        btn:RegisterForClicks("LeftButtonUp")
        btn:RegisterForDrag("LeftButton")
        btn.angle = math.pi * 0.75
        if type(GutSagaDB) == "table" and type(GutSagaDB.minimapAngle) == "number" then
            btn.angle = GutSagaDB.minimapAngle
        end
        -- Unread: soft blue ADD glow (Blizzard zoom highlight).
        local glow = btn:CreateTexture(nil, "BACKGROUND")
        glow:SetSize(58, 58)
        glow:SetPoint("CENTER")
        glow:SetTexture("Interface\\Minimap\\UI-Minimap-ZoomButton-Highlight")
        glow:SetBlendMode("ADD")
        glow:SetVertexColor(0.42, 0.72, 1.0)
        glow:SetAlpha(1)
        glow:Hide()
        btn.unreadGlow = glow
        -- LibDBIcon TrackingBorder layout, scaled so the icon stays ~30px
        -- (small holes look crunchy with the ornate gold rim).
        local icon = btn:CreateTexture(nil, "ARTWORK")
        icon:SetSize(30, 30)
        icon:SetPoint("TOPLEFT", 6, -5)
        icon:SetTexture("Interface\\AddOns\\GutSaga\\GutSagaMinimap")
        local border = btn:CreateTexture(nil, "OVERLAY")
        border:SetSize(66, 66)
        border:SetPoint("TOPLEFT")
        border:SetTexture("Interface\\Minimap\\MiniMap-TrackingBorder")
        local function placeMinimap(self)
            local angle = self.angle or (math.pi * 0.75)
            local radius = (Minimap:GetWidth() / 2) + 8
            self:ClearAllPoints()
            self:SetPoint("CENTER", Minimap, "CENTER", math.cos(angle) * radius, math.sin(angle) * radius)
        end
        placeMinimap(btn)
        btn:SetScript("OnDragStart", function(self)
            self.dragging = true
            self.moved = true
        end)
        btn:SetScript("OnDragStop", function(self)
            self.dragging = false
            initDB()
            GutSagaDB.minimapAngle = self.angle
        end)
        btn:SetScript("OnUpdate", function(self)
            if not self.dragging then
                return
            end
            local mx, my = Minimap:GetCenter()
            local scale = Minimap:GetEffectiveScale()
            if not mx or not scale or scale == 0 then
                return
            end
            local cx, cy = GetCursorPosition()
            cx, cy = cx / scale, cy / scale
            self.angle = math.atan2(cy - my, cx - mx)
            placeMinimap(self)
        end)
        btn:SetScript("OnClick", function(self)
            if self.moved then
                self.moved = false
                return
            end
            toggle()
        end)
        btn:SetScript("OnEnter", function(self)
            if not GameTooltip then
                return
            end
            GameTooltip:SetOwner(self, "ANCHOR_LEFT")
            GameTooltip:SetText("GutSaga")
            local unreadLine = unreadTooltipLine()
            if unreadLine then
                GameTooltip:AddLine(unreadLine, UNREAD_BLUE.r, UNREAD_BLUE.g, UNREAD_BLUE.b)
            end
            GameTooltip:AddLine("Dra for å flytte. Klikk for å åpne.", 0.9, 0.8, 0.6)
            GameTooltip:Show()
        end)
        btn:SetScript("OnLeave", function()
            if GameTooltip then
                GameTooltip:Hide()
            end
        end)
        updateMinimapUnread()
    end
end

SLASH_GUTSAGA1 = "/gutsaga"
SLASH_GUTSAGA2 = "/gs"
SlashCmdList.GUTSAGA = function(msg)
    msg = string.lower(string.gsub(msg or "", "^%s+", ""))
    if msg == "hide" then
        hide()
        return
    end
    if msg == "wipe" or msg == "clear" or msg == "reset" then
        -- Clear in-memory state and queue an API wipe. Companion deletes
        -- progress after /reload (SavedVariables flush). Editing the SV file
        -- on disk while logged in is overwritten on the next /reload.
        initDB()
        GutSagaDB.completions = {}
        GutSagaDB.objectives = {}
        markAllUnread()
        queueReset(nil)
        selectedID = nil
        chat("|cffc9a227GutSaga|r: Alle lokale fullføringer er slettet (ulest). Skriv |cffffffff/reload|r så companion rydder API-progress.")
        onWorld()
        if frame and frame:IsShown() then
            refresh()
        end
        return
    end
    if msg == "unread" then
        markAllUnread()
        chat("|cffc9a227GutSaga|r: Alle åpne sagaer er ulest.")
        updateMinimapUnread()
        if frame and frame:IsShown() then
            refresh()
        end
        return
    end
    local function resetQuest(id, label)
        initDB()
        GutSagaDB.completions[id] = nil
        if type(GutSagaDB.objectives) == "table" then
            GutSagaDB.objectives[id] = nil
        end
        local bucket = ensureReadMap()
        bucket[id] = nil
        queueReset(id)
        selectedID = id
        chat("|cffc9a227GutSaga|r: «" .. label .. "» er tilbakestilt. Skriv |cffffffff/reload|r så companion rydder API-progress.")
        onWorld()
        show()
    end
    if msg == "reset selected" or msg == "reset this" then
        if type(selectedID) == "string" and selectedID ~= "" then
            local label = selectedID
            local entries = GutSagaData and GutSagaData.entries
            if type(entries) == "table" then
                for _, e in ipairs(entries) do
                    if type(e) == "table" and e.quest_id == selectedID and type(e.title) == "string" then
                        label = e.title
                        break
                    end
                end
            end
            resetQuest(selectedID, label)
            return
        end
        chat("|cffc9a227GutSaga|r: Ingen saga er valgt. Åpne loggen og velg en, eller bruk |cffffffff/gutsaga wipe|r.")
        return
    end
    if msg == "reset goldshire" then
        resetQuest("cf031479-7288-4508-b139-f23f1e322026", "En kveld i Goldshire")
        return
    end
    if msg == "reset wilhelm" or msg == "reset salute" then
        resetQuest("88e92926-33b1-4ba0-a37b-6cb07ef6c7ba", "Hilsen til Wilhelm")
        return
    end
    if msg == "reset clap" or msg == "reset klapp" or msg == "reset trick" then
        resetQuest("59e95828-872b-46f1-9767-7d27e5e27fc5", "Klapp, så en frekk gest")
        return
    end
    if msg == "reset rabbits" or msg == "reset kaniner" then
        resetQuest("68a6b8dc-8cbf-4b36-9db0-dff4e6d605a0", "Kaniner i Elwynn")
        return
    end
    show()
end

-- Classic slash emotes always raise CHAT_MSG_TEXT_EMOTE, but DoEmote is not
-- always hooked (load order / client builds). Keep the last DoEmote token and
-- also infer common tokens from the localized chat line as a fallback.
local lastDoEmoteToken = nil

local function inferEmoteToken(msg)
    local s = plainString(msg)
    if not s then
        return nil
    end
    s = string.lower(s)
    -- English + Norwegian fragments for the emotes we author in sagas.
    local hints = {
        { "salute", "SALUTE" }, { "hils", "SALUTE" },
        { "bow", "BOW" }, { "bukk", "BOW" },
        { "wave", "WAVE" }, { "vink", "WAVE" },
        { "cheer", "CHEER" }, { "juble", "CHEER" },
        { "thank", "THANK" }, { "takk", "THANK" },
        { "applaud", "APPLAUD" }, { "applaus", "APPLAUD" }, { "bravo", "APPLAUD" },
        { "clap", "APPLAUD" }, { "klapp", "APPLAUD" },
        { "kneel", "KNEEL" }, { "knele", "KNEEL" },
        { "rude", "RUDE" }, { "frekk", "RUDE" }, { "ufin", "RUDE" },
        { "flex", "FLEX" },
        { "dance", "DANCE" }, { "dans", "DANCE" },
    }
    for i = 1, #hints do
        if string.find(s, hints[i][1], 1, true) then
            return hints[i][2]
        end
    end
    return nil
end

local function noteEmote(token, unit, msg)
    local emote = plainString(token)
    if not emote or emote == "" then
        return
    end
    emote = string.upper(emote)
    if string.sub(emote, 1, 1) == "/" then
        emote = string.sub(emote, 2)
    end
    emote = (emote:gsub("%s+", "_"))
    -- /clap and /applaud are different tokens; authors usually write APPLAUD.
    if emote == "CLAP" then
        emote = "APPLAUD"
    end
    if emote == "" then
        return
    end
    lastDoEmoteToken = emote
    readLocation()
    local unitID = plainString(unit)
    local target = ""
    if unitID and unitID ~= "" and unitID ~= "none" and unitID ~= "player" and UnitName then
        local ok, n = pcall(UnitName, unitID)
        target = (ok and plainString(n)) or ""
    end
    if target == "" and UnitName then
        local ok, n = pcall(UnitName, "target")
        target = (ok and plainString(n)) or ""
    end
    if target == "" and world.object and world.object ~= "" then
        target = world.object
    end
    -- Do not wipe a good target when CHAT_MSG_TEXT_EMOTE re-enters without one.
    if target == "" and type(world.pendingEmote) == "table" then
        local prev = plainString(world.pendingEmote.target)
        if prev then
            target = prev
        end
    end
    world.pendingEmote = {
        token = emote,
        target = target,
        text = plainString(msg) or "",
        zone = world.zone,
        subzone = world.subzone,
        minimap = world.minimap,
        mapID = world.mapID,
        x = world.x,
        y = world.y,
        untilAt = unixNow() + 5,
    }
end

local emoteHooked = false
local function installEmoteHook()
    if emoteHooked or type(hooksecurefunc) ~= "function" then
        return
    end
    local ok = pcall(function()
        hooksecurefunc("DoEmote", function(token, unit)
            noteEmote(token, unit)
            onWorld()
        end)
    end)
    if ok then
        emoteHooked = true
    end
end

local events = CreateFrame("Frame")
local tick = 0
events:RegisterEvent("ADDON_LOADED")
events:RegisterEvent("PLAYER_LOGIN")
events:RegisterEvent("PLAYER_ENTERING_WORLD")
events:RegisterEvent("PLAYER_LEVEL_UP")
events:RegisterEvent("CHAT_MSG_ADDON")
events:RegisterEvent("CHAT_MSG_TEXT_EMOTE")
events:RegisterEvent("ZONE_CHANGED")
events:RegisterEvent("ZONE_CHANGED_INDOORS")
events:RegisterEvent("ZONE_CHANGED_NEW_AREA")
events:RegisterEvent("GOSSIP_SHOW")
events:RegisterEvent("GOSSIP_CLOSED")
events:RegisterEvent("QUEST_DETAIL")
events:RegisterEvent("QUEST_PROGRESS")
events:RegisterEvent("QUEST_COMPLETE")
events:RegisterEvent("QUEST_FINISHED")
events:RegisterEvent("MERCHANT_SHOW")
events:RegisterEvent("MERCHANT_CLOSED")
events:RegisterEvent("TRAINER_SHOW")
events:RegisterEvent("TRAINER_CLOSED")
events:RegisterEvent("PLAYER_TARGET_CHANGED")
events:RegisterEvent("BAG_UPDATE")
events:RegisterEvent("GET_ITEM_INFO_RECEIVED")
events:RegisterEvent("COMBAT_LOG_EVENT_UNFILTERED")
events:SetScript("OnUpdate", function(_, elapsed)
    tick = tick + (elapsed or 0)
    if tick < 1 then
        return
    end
    tick = 0
    onWorld()
end)
events:SetScript("OnEvent", function(_, event, ...)
    if event == "ADDON_LOADED" then
        local name = ...
        if name ~= "GutSaga" then
            return
        end
        initDB()
        registerPrefix(PREFIX)
        installEmoteHook()
        return
    end
    if event == "PLAYER_LOGIN" or event == "PLAYER_ENTERING_WORLD" then
        initDB()
        installEmoteHook()
        showFreshNotice()
        cachePlayer()
        build()
        emitCharacter()
        onWorld()
        return
    end
    if event == "PLAYER_LEVEL_UP" then
        local newLevel = ...
        local n = plainNumber(newLevel)
        if not n then
            local ok, converted = pcall(tonumber, newLevel)
            if ok then
                n = plainNumber(converted)
            end
        end
        if n and n > 0 then
            player.level = n
        else
            cachePlayer()
        end
        emitCharacter()
        return
    end
    if event == "GOSSIP_SHOW" or event == "QUEST_DETAIL" or event == "QUEST_PROGRESS"
        or event == "QUEST_COMPLETE" or event == "MERCHANT_SHOW" or event == "TRAINER_SHOW" then
        readNPC()
        onWorld()
        return
    end
    if event == "GOSSIP_CLOSED" or event == "QUEST_FINISHED"
        or event == "MERCHANT_CLOSED" or event == "TRAINER_CLOSED" then
        clearNPC()
        return
    end
    if event == "PLAYER_TARGET_CHANGED" then
        readObject()
        onWorld()
        return
    end
    if event == "GET_ITEM_INFO_RECEIVED" then
        if frame and frame:IsShown() then
            refresh()
        end
        return
    end
    if event == "BAG_UPDATE" or event == "ZONE_CHANGED" or event == "ZONE_CHANGED_INDOORS" or event == "ZONE_CHANGED_NEW_AREA" then
        onWorld()
        return
    end
    if event == "COMBAT_LOG_EVENT_UNFILTERED" then
        -- Critters often never fire PARTY_KILL; count UNIT_DIED after we hit them.
        if CombatLogGetCurrentEventInfo then
            local ok, _, subevent, _, sourceGUID, _, _, _, destGUID, destName = pcall(CombatLogGetCurrentEventInfo)
            if ok and type(subevent) == "string" then
                local me = UnitGUID and UnitGUID("player")
                if sourceGUID and me and sourceGUID == me and destGUID and (
                    subevent == "SWING_DAMAGE" or subevent == "SPELL_DAMAGE" or subevent == "RANGE_DAMAGE"
                    or subevent == "SPELL_PERIODIC_DAMAGE" or subevent == "DAMAGE_SHIELD"
                ) then
                    world.lastHitGUID = destGUID
                    world.lastHitName = plainString(destName)
                end
                -- Critters may fire both PARTY_KILL and UNIT_DIED; count each GUID once.
                local recentUntil = tonumber(world.recentKillUntil) or 0
                if world.recentKillGUID and unixNow() > recentUntil then
                    world.recentKillGUID = nil
                    world.recentKillUntil = nil
                end
                local already = destGUID and world.recentKillGUID == destGUID and unixNow() <= recentUntil
                local counted = false
                if subevent == "PARTY_KILL" and destGUID and not already then
                    counted = true
                elseif subevent == "UNIT_DIED" and destGUID and not already and destGUID == world.lastHitGUID then
                    counted = true
                    if (not destName or destName == "") and world.lastHitName then
                        destName = world.lastHitName
                    end
                end
                if counted and destGUID then
                    world.recentKillGUID = destGUID
                    world.recentKillUntil = unixNow() + 3
                    world.lastHitGUID = nil
                end
                local witnessed = counted
                    and destName and not secret(destName) and plainString(destName)
                if witnessed then
                    world.deathName = witnessed
                end
                if counted then
                    if secret(destName) then
                        world.killSecret = true
                        world.killName = nil
                    else
                        world.killSecret = false
                        world.killName = plainString(destName)
                    end
                    if witnessed then
                        witnessGroupKills(witnessed)
                    end
                    onWorld()
                    world.killName = nil
                    world.killSecret = false
                    world.deathName = nil
                end
            end
        end
        return
    end
    if event == "CHAT_MSG_TEXT_EMOTE" then
        -- Prefer the chat line (correct for this emote) over lastDoEmoteToken,
        -- which can be stale after a wrong-emote negative test (/wave then /salute).
        local msg = ...
        local inferred = inferEmoteToken(msg)
        local token = inferred
        if (not token or token == "") and type(world.pendingEmote) == "table" then
            token = world.pendingEmote.token
        end
        if (not token or token == "") then
            token = lastDoEmoteToken
        end
        if token and token ~= "" then
            noteEmote(token, "target", msg)
            lastDoEmoteToken = nil
            onWorld()
        elseif type(world.pendingEmote) == "table" then
            world.pendingEmote.untilAt = unixNow() + 5
            onWorld()
        end
        return
    end
    if event == "CHAT_MSG_ADDON" then
        local prefix, message, _, sender = ...
        if prefix ~= PREFIX or type(sender) ~= "string" then
            return
        end
        local msg = plainString(message)
        local shortName = sender
        if Ambiguate then
            local ok, value = pcall(Ambiguate, sender, "short")
            if ok then
                local s = plainString(value)
                if s then
                    shortName = s
                end
            end
        end
        if msg and string.sub(msg, 1, 2) == "H\t" then
            if player.name ~= "" and same(shortName, player.name) then
                return
            end
            local qid, oid
            local first = string.find(msg, "\t", 3, true)
            local second = first and string.find(msg, "\t", first + 1, true)
            if first and second then
                qid = string.sub(msg, 3, first - 1)
                oid = string.sub(msg, first + 1, second - 1)
            elseif first then
                qid = string.sub(msg, 3, first - 1)
                oid = string.sub(msg, first + 1)
            end
            local group = readGroup()
            local known = false
            for _, m in ipairs(group.members or {}) do
                if type(m) == "table" and same(m.name, shortName) then
                    known = true
                    break
                end
            end
            if known and qid and oid and oid ~= "" then
                rememberAttest(qid, oid, shortName, unixNow())
                onWorld()
            end
            return
        end
        if msg and string.sub(msg, 1, 2) == "G\t" then
            local qid
            local tabAt = string.find(msg, "\t", 3, true)
            if tabAt then
                qid = string.sub(msg, 3, tabAt - 1)
            end
            if player.name ~= "" and same(shortName, player.name) then
                return
            end
            local entry
            for _, e in ipairs(currentEntries()) do
                if e.quest_id == qid then
                    entry = e
                    break
                end
            end
            local group = readGroup()
            local known = false
            for _, m in ipairs(group.members or {}) do
                if type(m) == "table" and same(m.name, shortName) then
                    known = true
                    break
                end
            end
            if entry and not isComplete(entry) and known and type(entry.group) == "table"
                and GutSagaObjectives and GutSagaObjectives.groupOK and GutSagaObjectives.groupOK(entry.group, group) then
                finishQuest(entry, group)
            end
            return
        end
        local short = sender
        if Ambiguate then
            local ok, value = pcall(Ambiguate, sender, "short")
            if ok then
                local s = plainString(value)
                if s then
                    short = s
                end
            end
        end
        if player.name ~= "" and same(short, player.name) then
            return
        end
        chat("|cffc9a227GutSaga|r: " .. short .. " fullførte en saga.")
    end
end)
