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
local GOLD = { r = 0.90, g = 0.70, b = 0.27 }
local listChild
local listScroll
local detailFS
local detailTitle
local detailChild
local detailScroll
local subtitle
local rows = {}
local selectedID
local built = false
local checking = false
local lastCharacterLine = ""

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

local function bullets(text)
    if type(text) ~= "string" or text == "" then
        return "Ingen"
    end
    local lines = {}
    for line in string.gmatch(text, "[^\n]+") do
        table.insert(lines, "• " .. line)
    end
    if #lines == 0 then
        return "Ingen"
    end
    return table.concat(lines, "\n")
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

local function objectiveText(entry)
    local objs = entry.objectives
    if type(objs) ~= "table" then
        return bullets(type(objs) == "string" and objs or "")
    end
    local bucket = savedSteps(entry)
    local lines = {}
    local locked = false
    for i, obj in ipairs(objs) do
        if type(obj) == "table" then
            local id = obj.id or tostring(i)
            local step = bucket.steps[id] or {}
            local mark = "|cff888888-|r"
            if step.done then
                mark = "|cff20ff20+|r"
            elseif locked then
                mark = "|cff666666-|r"
            else
                mark = "|cffffffff>|r"
            end
            if not step.done then
                locked = true
            end
            local label = applyItemLink(obj.text or "Mål", obj)
            if obj.type == "location" and not step.done then
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
            if (obj.type == "collect" or obj.type == "kill") and not step.done then
                local need = tonumber(obj.count) or 1
                local have = tonumber(step.count) or 0
                if obj.type == "collect" then
                    local id = tonumber(obj.item_id)
                    if id then
                        local live = itemCount(id)
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
            table.insert(lines, mark .. " " .. label)
        end
    end
    if #lines == 0 then
        return "Ingen"
    end
    return table.concat(lines, "\n")
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
    row:SetScript("OnClick", function(self)
        selectedID = self.questID
        refresh()
    end)
    rows[i] = row
    return row
end

local function placeSectionHeader(header, y, label, expanded, onToggle)
    header:ClearAllPoints()
    header:SetPoint("TOPLEFT", listChild, "TOPLEFT", 0, -y)
    header:SetPoint("TOPRIGHT", listChild, "TOPRIGHT", 0, -y)
    header:Show()
    local arrow = expanded and "[-]" or "[+]"
    header.label:SetText(arrow .. "  " .. label)
    header:SetScript("OnClick", onToggle)
    return y + 24
end

refresh = function()
    if not built then
        return
    end
    local entries = currentEntries()
    if player.name == "" then
        subtitle:SetText("")
    else
        subtitle:SetText(player.name)
    end

    local openList, doneList = {}, {}
    for i = 1, #entries do
        if isComplete(entries[i]) then
            table.insert(doneList, entries[i])
        else
            table.insert(openList, entries[i])
        end
    end

    local openExpanded = sectionExpanded("open", true)
    local doneExpanded = sectionExpanded("done", true)
    local y = 0
    local rowIndex = 0

    y = placeSectionHeader(sectionOpen, y, "Ikke startet (" .. #openList .. ")", openExpanded, function()
        setSectionExpanded("open", not openExpanded)
        refresh()
    end)
    if openExpanded then
        for i = 1, #openList do
            rowIndex = rowIndex + 1
            local row = ensureRow(rowIndex)
            local entry = openList[i]
            row:ClearAllPoints()
            row:SetPoint("TOPLEFT", listChild, "TOPLEFT", 4, -y)
            row:Show()
            row.questID = entry.quest_id
            row.title:SetText(entry.title or "Saga")
            row.title:SetTextColor(GOLD.r, GOLD.g, GOLD.b)
            row.sub:SetText(creatorName(entry))
            row.sub:SetTextColor(0.72, 0.68, 0.58)
            if entry.quest_id == selectedID then
                row.bg:SetAlpha(0.7)
            else
                row.bg:SetAlpha(0)
            end
            y = y + 40
        end
    end

    y = y + 6
    y = placeSectionHeader(sectionDone, y, "Fullførte (" .. #doneList .. ")", doneExpanded, function()
        setSectionExpanded("done", not doneExpanded)
        refresh()
    end)
    if doneExpanded then
        for i = 1, #doneList do
            rowIndex = rowIndex + 1
            local row = ensureRow(rowIndex)
            local entry = doneList[i]
            row:ClearAllPoints()
            row:SetPoint("TOPLEFT", listChild, "TOPLEFT", 4, -y)
            row:Show()
            row.questID = entry.quest_id
            row.title:SetText(entry.title or "Saga")
            row.title:SetTextColor(0.55, 0.75, 0.45)
            row.sub:SetText(creatorName(entry))
            row.sub:SetTextColor(0.55, 0.52, 0.45)
            if entry.quest_id == selectedID then
                row.bg:SetAlpha(0.55)
            else
                row.bg:SetAlpha(0)
            end
            y = y + 40
        end
    end

    for i = rowIndex + 1, #rows do
        rows[i]:Hide()
    end
    listChild:SetHeight(math.max(y + 8, 10))

    local entry = selectedID and findEntry(selectedID) or nil
    if not entry then
        if openList[1] then
            entry = openList[1]
        elseif doneList[1] then
            entry = doneList[1]
        end
        if entry then
            selectedID = entry.quest_id
        end
    end
    local width = detailScroll:GetWidth() - 12
    if not entry then
        detailTitle:SetText("")
        if player.name == "" then
            detailFS:SetText("Logg inn på en karakter.")
        elseif type(GutSagaData) ~= "table" or type(GutSagaData.entries) ~= "table" or #GutSagaData.entries == 0 then
            detailFS:SetText("Ingen sagaer ennå.\n\nStart GutSaga på maskinen og skriv /reload.")
        else
            detailFS:SetText("Ingen sagaer for " .. player.name .. ".")
        end
    else
        local description = (type(entry.description) == "string" and entry.description ~= "") and entry.description or "Ingen"
        detailTitle:SetText(entry.title or "Saga")
        detailFS:SetText(table.concat({
            "|cff3f2a14Beskrivelse|r",
            description,
            "",
            "|cff3f2a14Mål|r",
            objectiveText(entry),
        }, "\n"))
    end
    detailTitle:SetWidth(width)
    detailFS:SetWidth(width)
    local titleH = detailTitle:GetStringHeight() or 0
    if detailTitle:GetText() == "" then
        titleH = 0
        detailFS:SetPoint("TOPLEFT", detailChild, "TOPLEFT", 6, -6)
    else
        detailFS:SetPoint("TOPLEFT", detailTitle, "BOTTOMLEFT", 0, -10)
    end
    local height = titleH + (detailFS:GetStringHeight() or 10) + 24
    detailChild:SetHeight(math.max(height, 10))
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
    showSagaToast(entry.title or "Saga")
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

local function scrollBy(self, delta, step)
    local nxt = self:GetVerticalScroll() - delta * step
    local maxScroll = self:GetVerticalScrollRange()
    if nxt < 0 then
        nxt = 0
    end
    if nxt > maxScroll then
        nxt = maxScroll
    end
    self:SetVerticalScroll(nxt)
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
    local label = btn:CreateFontString(nil, "OVERLAY", "GameFontNormal")
    label:SetPoint("LEFT", 6, 0)
    label:SetTextColor(GOLD.r, GOLD.g, GOLD.b)
    btn.label = label
    return btn
end

local function build()
    if built then
        return
    end
    built = true
    local backdropTemplate = BackdropTemplateMixin and "BackdropTemplate" or nil
    frame = CreateFrame("Frame", "GutSagaFrame", UIParent, backdropTemplate)
    frame:SetSize(720, 500)
    frame:SetPoint("CENTER")
    frame:SetFrameStrata("HIGH")
    frame:SetMovable(true)
    frame:SetClampedToScreen(true)
    frame:EnableMouse(true)
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
    title:SetTextColor(1, 0.92, 0.65)
    subtitle = frame:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    subtitle:SetPoint("TOP", frame, "TOP", 0, -34)
    subtitle:SetTextColor(0.92, 0.82, 0.55)

    local mark = frame:CreateTexture(nil, "OVERLAY")
    mark:SetSize(58, 58)
    mark:SetPoint("TOPLEFT", frame, "TOPLEFT", -10, 14)
    mark:SetTexture("Interface\\AddOns\\GutSaga\\GutSaga")

    local close = CreateFrame("Button", nil, frame, "UIPanelCloseButton")
    close:SetPoint("TOPRIGHT", frame, "TOPRIGHT", -2, -2)

    local drag = CreateFrame("Frame", nil, frame)
    drag:SetPoint("TOPLEFT", frame, "TOPLEFT", 56, 10)
    drag:SetPoint("TOPRIGHT", frame, "TOPRIGHT", -46, 10)
    drag:SetHeight(36)
    drag:EnableMouse(true)
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
    listPanel:SetWidth(236)
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

    listScroll = CreateFrame("ScrollFrame", nil, listPanel)
    listScroll:SetPoint("TOPLEFT", 6, -6)
    listScroll:SetPoint("BOTTOMRIGHT", -6, 6)
    listChild = CreateFrame("Frame", nil, listScroll)
    listChild:SetSize(210, 360)
    listScroll:SetScrollChild(listChild)
    listScroll:EnableMouseWheel(true)
    listScroll:SetScript("OnMouseWheel", function(self, delta)
        scrollBy(self, delta, 36)
    end)
    sectionOpen = makeSectionHeader(listChild)
    sectionDone = makeSectionHeader(listChild)

    detailPanel = CreateFrame("Frame", nil, frame, backdropTemplate)
    detailPanel:SetPoint("TOPLEFT", listPanel, "TOPRIGHT", 14, 0)
    detailPanel:SetPoint("BOTTOMLEFT", listPanel, "BOTTOMRIGHT", 14, 0)
    detailPanel:SetWidth(400)
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
    parchment:SetTexCoord(0, 0.72, 0.02, 0.72)
    parchment:SetVertexColor(1, 0.97, 0.9)

    detailScroll = CreateFrame("ScrollFrame", nil, detailPanel)
    detailScroll:SetPoint("TOPLEFT", 12, -12)
    detailScroll:SetPoint("BOTTOMRIGHT", -12, 12)
    detailChild = CreateFrame("Frame", nil, detailScroll)
    detailChild:SetSize(360, 200)
    detailScroll:SetScrollChild(detailChild)
    detailScroll:EnableMouseWheel(true)
    detailScroll:SetScript("OnMouseWheel", function(self, delta)
        scrollBy(self, delta, 28)
    end)
    local titleFont = "GameFontNormalLarge"
    if QuestTitleFont then
        titleFont = "QuestTitleFont"
    end
    detailTitle = detailChild:CreateFontString(nil, "OVERLAY", titleFont)
    detailTitle:SetPoint("TOPLEFT", 6, -4)
    detailTitle:SetJustifyH("LEFT")
    detailTitle:SetJustifyV("TOP")
    detailTitle:SetWordWrap(true)
    detailTitle:SetTextColor(0.22, 0.12, 0.04)
    detailFS = detailChild:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    detailFS:SetPoint("TOPLEFT", detailTitle, "BOTTOMLEFT", 0, -10)
    detailFS:SetJustifyH("LEFT")
    detailFS:SetJustifyV("TOP")
    detailFS:SetWordWrap(true)
    detailFS:SetSpacing(2)
    detailFS:SetTextColor(0.22, 0.12, 0.04)
    if detailFS.SetHyperlinksEnabled then
        detailFS:SetHyperlinksEnabled(true)
    end
    detailChild:EnableMouse(true)
    local function linkClick(_, link, text, button)
        if IsModifiedClick and IsModifiedClick("DRESSUP") and DressUpItemLink then
            pcall(DressUpItemLink, link)
            return
        end
        if SetItemRef then
            pcall(SetItemRef, link, text, button or "LeftButton")
        end
    end
    local function linkEnter(owner, link)
        if not GameTooltip or type(link) ~= "string" then
            return
        end
        GameTooltip:SetOwner(owner, "ANCHOR_CURSOR")
        local ok = pcall(GameTooltip.SetHyperlink, GameTooltip, link)
        if ok then
            GameTooltip:Show()
        end
    end
    local function linkLeave()
        if GameTooltip then
            GameTooltip:Hide()
        end
    end
    pcall(function()
        detailFS:SetScript("OnHyperlinkClick", linkClick)
        detailFS:SetScript("OnHyperlinkEnter", linkEnter)
        detailFS:SetScript("OnHyperlinkLeave", linkLeave)
    end)

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
    showSagaToast = function(titleText)
        toastTitle:SetText(titleText or "Saga")
        toast.age = 0
        toast:SetAlpha(0)
        toast:Show()
        if PlaySound then
            local played = false
            if SOUNDKIT and SOUNDKIT.ACHIEVEMENT_EARNED then
                played = pcall(PlaySound, SOUNDKIT.ACHIEVEMENT_EARNED)
            end
            if not played then
                pcall(PlaySound, 888)
            end
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
    msg = string.lower(msg or "")
    if msg == "hide" then
        hide()
    else
        show()
    end
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
