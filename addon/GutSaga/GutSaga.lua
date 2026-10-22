-- GutSaga quest log for Forever, retail Midnight, Classic Era, and Classic (Mists).
-- Interface numbers live in the .toc files (see addon/INTERFACE).
-- Machine-readable chat lines (ASCII, parsed by the companion):
--   [GutSaga] CHARACTER name|realm|level|race|class|battletag|unix
--   [GutSaga] COMPLETE <questId> <version> <character> <realm words> <level> <unix>
-- Realm in COMPLETE may contain spaces. Empty fields in CHARACTER are "-".
-- There is no completion button. Objectives are checked in order.

local PREFIX = "GutSaga"
local player = { name = "", realm = "", level = 1, race = "", class = "", battletag = "" }
local world = { zone = "", subzone = "", minimap = "", mapID = nil, x = nil, y = nil, npc = "", object = "", killName = nil, killSecret = false }
local frame
local toast
local showSagaToast = function() end
local sectionOpen
local sectionDone
local listPanel
local detailPanel
local GOLD = { r = 0.99, g = 0.93, b = 0.59 }
local listChild
local listScroll
local detailFS
local detailTitle
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

local function chat(line)
    -- Visible in the chat UI. Classic Era does not write AddMessage into
    -- WoWChatLog.txt; the companion also reads GutSagaDB.completions.
    if DEFAULT_CHAT_FRAME and DEFAULT_CHAT_FRAME.AddMessage then
        DEFAULT_CHAT_FRAME:AddMessage(line)
    else
        print(line)
    end
end

local function enableChatLog()
    if C_CVar and C_CVar.SetCVar then
        pcall(C_CVar.SetCVar, "chatLog", "1")
    elseif SetCVar then
        pcall(SetCVar, "chatLog", "1")
    end
    if LoggingChat then
        pcall(LoggingChat, true)
    end
end

local function takeScreenshot()
    if Screenshot then
        pcall(Screenshot)
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

local function activityLine(row)
    if type(row) ~= "table" then
        return ""
    end
    local name = plainString(row.character) or ""
    if name == "" then
        return ""
    end
    local tag = plainString(row.battletag)
    if tag and tag ~= "" then
        return name .. " — " .. tag
    end
    local realm = plainString(row.realm)
    if realm and realm ~= "" then
        return name .. " — " .. realm
    end
    return name
end

local function activityList(entry)
    local out = {}
    local seen = {}
    local function add(row)
        local line = activityLine(row)
        if line == "" then
            return
        end
        local key = line:lower()
        if seen[key] then
            return
        end
        seen[key] = true
        local at = ""
        if type(row) == "table" then
            if type(row.completed_at) == "string" then
                at = row.completed_at
            elseif type(row.completed_at) == "number" then
                at = string.format("%014d", row.completed_at)
            end
        end
        table.insert(out, { text = line, at = at })
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
                completed_at = localDone.completed_at,
            })
        end
    end
    table.sort(out, function(a, b)
        if a.at ~= b.at then
            return a.at < b.at
        end
        return a.text < b.text
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
    for i, obj in ipairs(objs) do
        if type(obj) == "table" then
            local id = obj.id or tostring(i)
            local step = bucket.steps[id] or {}
            local done = step.done and true or false
            local label
            if done then
                label = (obj.text or "Mål") .. " (Fullført)"
            else
                label = applyItemLink(obj.text or "Mål", obj)
                if obj.type == "location" then
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
                if obj.type == "collect" or obj.type == "kill" then
                    local need = tonumber(obj.count) or 1
                    local have = tonumber(step.count) or 0
                    if obj.type == "collect" then
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
                if obj.type == "kill" then
                    label = label .. " (eksperimentell)"
                end
            end
            table.insert(lines, { done = done, text = label })
        end
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
    end
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

local function objectiveState(entries)
    local items = {}
    for _, entry in ipairs(entries) do
        if type(entry.objectives) == "table" and not isComplete(entry) then
            for _, obj in ipairs(entry.objectives) do
                if type(obj) == "table" and obj.type == "collect" then
                    local id = tonumber(obj.item_id)
                    if id and items[id] == nil then
                        items[id] = itemCount(id)
                    end
                end
            end
        end
    end
    return {
        zone = world.zone,
        subzone = world.subzone,
        minimap = world.minimap,
        map_id = world.mapID,
        x = world.x,
        y = world.y,
        npc = world.npc,
        object = world.object,
        items = items,
        kill_name = world.killName,
        kill_secret = world.killSecret,
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
        for _, entry in ipairs(entries) do
            if not isComplete(entry) and type(entry.objectives) == "table" and #entry.objectives > 0 then
                local bucket = savedSteps(entry)
                local steps, done = GutSagaObjectives.evaluate(entry.objectives, state, bucket.steps)
                bucket.steps = steps
                if done then
                    finishQuest(entry)
                end
            end
        end
        world.killName = nil
        world.killSecret = false
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

local function ensureActivityRow(i)
    local row = activityRows[i]
    if row then
        return row
    end
    row = CreateFrame("Frame", nil, detailChild)
    row:SetHeight(16)
    local text = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    text:SetPoint("TOPLEFT", 0, 0)
    text:SetPoint("TOPRIGHT", 0, 0)
    text:SetJustifyH("LEFT")
    text:SetJustifyV("TOP")
    text:SetWordWrap(true)
    text:SetTextColor(INK.r, INK.g, INK.b)
    row.text = text
    bindWheel(row, detailScroll, 28)
    bindWheel(text, detailScroll, 28)
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
    local title = row:CreateFontString(nil, "OVERLAY", "GameFontNormal")
    title:SetPoint("TOPLEFT", 8, -4)
    title:SetPoint("TOPRIGHT", -8, -4)
    title:SetJustifyH("LEFT")
    title:SetWordWrap(false)
    row.title = title
    local sub = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    sub:SetPoint("BOTTOMLEFT", 8, 4)
    sub:SetPoint("BOTTOMRIGHT", -8, 4)
    sub:SetJustifyH("LEFT")
    row.sub = sub
    bindWheel(row, listScroll, 36)
    bindWheel(title, listScroll, 36)
    bindWheel(sub, listScroll, 36)
    row:SetScript("OnClick", function(self)
        selectedID = self.questID
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
            row.title:SetTextColor(GOLD.r, GOLD.g, GOLD.b)
            row.sub:SetText(creatorName(item))
            row.sub:SetTextColor(0.72, 0.68, 0.58)
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
            row.title:SetTextColor(0.55, 0.75, 0.45)
            row.sub:SetText(creatorName(item))
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
        if detailActivityHeader then
            detailActivityHeader:Hide()
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
        local description = formatDescription(selectedEntry.description)
        detailTitle:Show()
        detailTitle:SetText(selectedEntry.title or "Saga")
        detailTitle:SetTextColor(INK.r, INK.g, INK.b)
        detailTitle:SetWidth(width)
        detailTitle:ClearAllPoints()
        detailTitle:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailY = detailY + (detailTitle:GetStringHeight() or 20) + 12

        detailFS:SetText(description)
        detailFS:SetTextColor(INK.r, INK.g, INK.b)
        detailFS:SetSpacing(4)
        detailFS:SetWidth(width)
        detailFS:ClearAllPoints()
        detailFS:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailY = detailY + (detailFS:GetStringHeight() or 14) + 16

        detailGoalsHeader:Show()
        detailGoalsHeader:ClearAllPoints()
        detailGoalsHeader:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        detailGoalsHeader:SetWidth(width)
        detailGoalsHeader:SetTextColor(INK.r, INK.g, INK.b)
        detailY = detailY + (detailGoalsHeader:GetStringHeight() or 20) + 10

        local okGoals, goalList = pcall(objectiveLines, selectedEntry)
        if okGoals and type(goalList) == "table" then
            goals = goalList
        end
    end

    local textWidth = width - 22
    for i, goal in ipairs(goals) do
        local row = ensureGoalRow(i)
        row:ClearAllPoints()
        row:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        row:SetWidth(width)
        row:Show()
        if goal.done then
            row.box:SetTexture("Interface\\AddOns\\GutSaga\\check")
            row.box:SetVertexColor(INK.r, INK.g, INK.b)
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
        local row = ensureActivityRow(i)
        row:ClearAllPoints()
        row:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -detailY)
        row:SetWidth(width)
        row:Show()
        row.text:SetWidth(width)
        row.text:SetText(act.text or "")
        row.text:SetTextColor(INK.r, INK.g, INK.b)
        local h = math.max(14, (row.text:GetStringHeight() or 12) + 2)
        row:SetHeight(h)
        detailY = detailY + h + 6
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
end

finishQuest = function(entry)
    if not entry or isComplete(entry) then
        return
    end
    cachePlayer()
    initDB()
    if player.name == "" then
        return
    end
    local version = tonumber(entry.quest_version) or 0
    local level = player.level
    if level < 1 then
        level = 1
    end
    local ts = unixNow()
    GutSagaDB.completions[entry.quest_id] = {
        quest_id = entry.quest_id,
        version = version,
        level = level,
        completed_at = ts,
        character = player.name,
        realm = player.realm,
    }
    takeScreenshot()
    local realm = player.realm
    if realm == "" then
        realm = "-"
    end
    chat(string.format("[GutSaga] COMPLETE %s %d %s %s %d %d", entry.quest_id, version, player.name, realm, level, ts))
    showSagaToast(entry.title or "Saga", entry.quest_id)
    broadcast(string.format("C|%s|%d|%d|%d", entry.quest_id, version, level, ts))
    refresh()
end

local function onWorld()
    readLocation()
    readNPC()
    readObject()
    checkAll()
    if frame and frame:IsShown() then
        refresh()
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
    detailGoalsHeader = detailChild:CreateFontString(nil, "OVERLAY", titleFont)
    detailGoalsHeader:SetJustifyH("LEFT")
    detailGoalsHeader:SetText("Mål")
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
    toast:SetScript("OnUpdate", function(self, elapsed)
        if not self:IsShown() then
            return
        end
        self.age = (self.age or 0) + (elapsed or 0)
        local age = self.age
        local alpha, y = 1, -30
        if age < 0.35 then
            local t = age / 0.35
            alpha = t
            y = 16 - 46 * t
        elseif age < 4.2 then
            alpha = 1
            y = -30
        elseif age < 5 then
            local t = (age - 4.2) / 0.8
            alpha = 1 - t
            y = -30 - 18 * t
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
        btn:SetSize(32, 32)
        btn:SetFrameStrata("MEDIUM")
        btn:RegisterForClicks("LeftButtonUp")
        btn:RegisterForDrag("LeftButton")
        btn.angle = math.pi * 0.75
        if type(GutSagaDB) == "table" and type(GutSagaDB.minimapAngle) == "number" then
            btn.angle = GutSagaDB.minimapAngle
        end
        local icon = btn:CreateTexture(nil, "ARTWORK")
        icon:SetAllPoints()
        icon:SetTexture("Interface\\AddOns\\GutSaga\\GutSaga")
        local function placeMinimap(self)
            local angle = self.angle or (math.pi * 0.75)
            local radius = (Minimap:GetWidth() / 2) + 5
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
            GameTooltip:AddLine("Dra for å flytte. Klikk for å åpne.", 0.9, 0.8, 0.6)
            GameTooltip:Show()
        end)
        btn:SetScript("OnLeave", function()
            if GameTooltip then
                GameTooltip:Hide()
            end
        end)
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
    if msg == "reset" or msg == "reset goldshire" then
        initDB()
        local id = "cf031479-7288-4508-b139-f23f1e322026"
        GutSagaDB.completions[id] = nil
        GutSagaDB.objectives[id] = nil
        selectedID = id
        chat("|cffc9a227GutSaga|r: «En kveld i Goldshire» er tilbakestilt.")
        onWorld()
        show()
        return
    end
    show()
end

local events = CreateFrame("Frame")
local tick = 0
events:RegisterEvent("ADDON_LOADED")
events:RegisterEvent("PLAYER_LOGIN")
events:RegisterEvent("PLAYER_ENTERING_WORLD")
events:RegisterEvent("PLAYER_LEVEL_UP")
events:RegisterEvent("CHAT_MSG_ADDON")
events:RegisterEvent("ZONE_CHANGED")
events:RegisterEvent("ZONE_CHANGED_INDOORS")
events:RegisterEvent("ZONE_CHANGED_NEW_AREA")
events:RegisterEvent("GOSSIP_SHOW")
events:RegisterEvent("QUEST_DETAIL")
events:RegisterEvent("QUEST_PROGRESS")
events:RegisterEvent("QUEST_COMPLETE")
events:RegisterEvent("MERCHANT_SHOW")
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
        enableChatLog()
        registerPrefix(PREFIX)
        return
    end
    if event == "PLAYER_LOGIN" or event == "PLAYER_ENTERING_WORLD" then
        initDB()
        enableChatLog()
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
        or event == "QUEST_COMPLETE" or event == "MERCHANT_SHOW" then
        readNPC()
        onWorld()
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
        if CombatLogGetCurrentEventInfo then
            local ok, _, subevent, _, _, _, _, _, _, destName = pcall(CombatLogGetCurrentEventInfo)
            if ok and subevent == "PARTY_KILL" then
                if secret(destName) then
                    world.killSecret = true
                    world.killName = nil
                else
                    world.killSecret = false
                    world.killName = plainString(destName)
                end
                onWorld()
            end
        end
        return
    end
    if event == "CHAT_MSG_ADDON" then
        local prefix, _, _, sender = ...
        if prefix ~= PREFIX or type(sender) ~= "string" then
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
