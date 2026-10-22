-- GutSaga quest log for World of Warcraft: Forever and retail Midnight.
-- Interface numbers live in the .toc files (see addon/INTERFACE).
-- Machine-readable chat lines (ASCII, parsed by the companion):
--   [GutSaga] CHARACTER name|realm|level|race|class|battletag|unix
--   [GutSaga] COMPLETE <questId> <version> <character> <realm words> <level> <unix>
-- Realm in COMPLETE may contain spaces. Empty fields in CHARACTER are "-".
-- There is no completion button. Objectives are checked in order.

local PREFIX = "GutSaga"
local player = { name = "", realm = "", level = 1, race = "", class = "", battletag = "" }
local world = { zone = "", subzone = "", mapID = nil, x = nil, y = nil, npc = "", object = "", killName = nil, killSecret = false }
local frame
local listChild
local listScroll
local detailFS
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
    end
    if LoggingChat then
        pcall(LoggingChat, true)
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

local function statusText(entry)
    if isComplete(entry) then
        return "Fullført"
    end
    if entry.status == "active" then
        return "Aktiv"
    end
    if entry.status == "offered" then
        return "Tilbudt"
    end
    return "Ikke startet"
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
            local mark = "·"
            if step.done then
                mark = "✓"
            elseif locked then
                mark = "–"
            end
            if not step.done then
                locked = true
            end
            local label = obj.text or "Mål"
            if (obj.type == "collect" or obj.type == "kill") and not step.done then
                local need = tonumber(obj.count) or 1
                local have = tonumber(step.count) or 0
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
    if inInstance() or not C_ChatInfo or not C_ChatInfo.SendAddonMessage then
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
        pcall(C_ChatInfo.SendAddonMessage, PREFIX, message, "RAID")
    elseif inGroup then
        pcall(C_ChatInfo.SendAddonMessage, PREFIX, message, "PARTY")
    end
    if inGuild then
        pcall(C_ChatInfo.SendAddonMessage, PREFIX, message, "GUILD")
    end
end

local function itemCount(id)
    if C_Item and C_Item.GetItemCount then
        local ok, n = pcall(C_Item.GetItemCount, id)
        local v = ok and plainNumber(n) or nil
        if v then
            return v
        end
    end
    if GetItemCount then
        local ok, n = pcall(GetItemCount, id)
        local v = ok and plainNumber(n) or nil
        if v then
            return v
        end
    end
    return 0
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
    if not C_Map or not C_Map.GetBestMapForUnit then
        return
    end
    local ok, mapID = pcall(C_Map.GetBestMapForUnit, "player")
    local id = ok and plainNumber(mapID) or nil
    if not id or not C_Map.GetPlayerMapPosition then
        return
    end
    world.mapID = id
    local posOK, a, b = pcall(C_Map.GetPlayerMapPosition, id, "player")
    if not posOK then
        return
    end
    local x, y
    if type(a) == "number" and type(b) == "number" then
        x, y = a, b
    elseif type(a) == "table" then
        x, y = a.x, a.y
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
    checking = false
end

refresh = function()
    if not built then
        return
    end
    local entries = currentEntries()
    local syncAt = ""
    if type(GutSagaData) == "table" and type(GutSagaData.generated_at) == "string" then
        syncAt = GutSagaData.generated_at
    end
    if player.name == "" then
        subtitle:SetText("Venter på karakterinformasjon")
    elseif syncAt ~= "" then
        subtitle:SetText(player.name .. "  ·  sist synkronisert " .. syncAt)
    else
        subtitle:SetText(player.name .. "  ·  ingen oppdragsdata ennå")
    end
    for i = 1, #entries do
        local row = rows[i]
        if not row then
            row = CreateFrame("Button", nil, listChild)
            row:SetSize(220, 36)
            row:SetPoint("TOPLEFT", 0, -((i - 1) * 38))
            local bg = row:CreateTexture(nil, "BACKGROUND")
            bg:SetAllPoints()
            row.bg = bg
            local title = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
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
        end
        local entry = entries[i]
        row.questID = entry.quest_id
        row:Show()
        row.title:SetText(entry.title or "Oppdrag")
        row.sub:SetText(statusText(entry))
        if entry.quest_id == selectedID then
            row.bg:SetColorTexture(0.42, 0.3, 0.08, 1)
        elseif isComplete(entry) then
            row.bg:SetColorTexture(0.12, 0.16, 0.12, 1)
        else
            row.bg:SetColorTexture(0.16, 0.13, 0.1, 1)
        end
    end
    for i = #entries + 1, #rows do
        rows[i]:Hide()
    end
    listChild:SetHeight(math.max(#entries * 38, 10))

    local entry = selectedID and findEntry(selectedID) or nil
    if not entry and entries[1] then
        entry = entries[1]
        selectedID = entry.quest_id
    end
    if not entry then
        if player.name == "" then
            detailFS:SetText("Logg inn på en karakter.")
        elseif type(GutSagaData) ~= "table" or type(GutSagaData.entries) ~= "table" or #GutSagaData.entries == 0 then
            detailFS:SetText("Ingen oppdragsdata.\n\nStart GutSaga på maskinen, logg inn med Smia, og skriv /reload.")
        else
            detailFS:SetText("Ingen oppdrag for " .. player.name .. ".\n\nNye oppdrag vises etter synkronisering og /reload.")
        end
    else
        local minLevel = tonumber(entry.min_level) and tostring(entry.min_level) or "–"
        local description = (type(entry.description) == "string" and entry.description ~= "") and entry.description or "Ingen"
        local reward = (type(entry.reward) == "string" and entry.reward ~= "") and entry.reward or "Ingen"
        detailFS:SetText(table.concat({
            entry.title or "Oppdrag",
            "",
            "Status: " .. statusText(entry),
            "Versjon: " .. tostring(tonumber(entry.quest_version) or 0),
            "Minste nivå: " .. minLevel,
            "",
            "Beskrivelse",
            description,
            "",
            "Mål",
            objectiveText(entry),
            "",
            "Belønning",
            reward,
        }, "\n"))
    end
    detailFS:SetWidth(detailScroll:GetWidth() - 8)
    local height = detailFS:GetStringHeight()
    detailChild:SetHeight(math.max((height or 10) + 16, 10))
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
    if Screenshot then
        pcall(Screenshot)
    end
    local realm = player.realm
    if realm == "" then
        realm = "-"
    end
    chat(string.format("[GutSaga] COMPLETE %s %d %s %s %d %d", entry.quest_id, version, player.name, realm, level, ts))
    chat("|cffc9a227GutSaga|r: Fullført – " .. (entry.title or "oppdrag"))
    broadcast(string.format("C|%s|%d|%d|%d", entry.quest_id, version, level, ts))
    refresh()
end

local function onWorld()
    readLocation()
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

local function build()
    if built then
        return
    end
    built = true
    frame = CreateFrame("Frame", "GutSagaFrame", UIParent)
    frame:SetSize(720, 480)
    frame:SetPoint("CENTER")
    frame:SetFrameStrata("HIGH")
    frame:SetMovable(true)
    frame:SetClampedToScreen(true)
    frame:Hide()
    table.insert(UISpecialFrames, "GutSagaFrame")

    local bg = frame:CreateTexture(nil, "BACKGROUND")
    bg:SetAllPoints()
    bg:SetColorTexture(0.07, 0.06, 0.05, 0.96)
    local bar = frame:CreateTexture(nil, "BORDER")
    bar:SetPoint("TOPLEFT")
    bar:SetPoint("TOPRIGHT")
    bar:SetHeight(44)
    bar:SetColorTexture(0.30, 0.20, 0.07, 1)

    local drag = CreateFrame("Frame", nil, frame)
    drag:SetPoint("TOPLEFT")
    drag:SetPoint("TOPRIGHT", -28, 0)
    drag:SetHeight(44)
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

    local title = frame:CreateFontString(nil, "OVERLAY", "GameFontNormalLarge")
    title:SetPoint("TOPLEFT", 16, -8)
    title:SetText("GutSaga")
    title:SetTextColor(1, 0.86, 0.45)
    subtitle = frame:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    subtitle:SetPoint("TOPLEFT", 110, -14)
    subtitle:SetTextColor(0.9, 0.82, 0.65)

    local close = CreateFrame("Button", nil, frame, "UIPanelCloseButton")
    close:SetPoint("TOPRIGHT", 0, 0)

    listScroll = CreateFrame("ScrollFrame", nil, frame)
    listScroll:SetPoint("TOPLEFT", 12, -56)
    listScroll:SetSize(230, 360)
    listChild = CreateFrame("Frame", nil, listScroll)
    listChild:SetSize(230, 360)
    listScroll:SetScrollChild(listChild)
    listScroll:EnableMouseWheel(true)
    listScroll:SetScript("OnMouseWheel", function(self, delta)
        local nxt = self:GetVerticalScroll() - delta * 36
        local maxScroll = self:GetVerticalScrollRange()
        if nxt < 0 then
            nxt = 0
        end
        if nxt > maxScroll then
            nxt = maxScroll
        end
        self:SetVerticalScroll(nxt)
    end)

    detailScroll = CreateFrame("ScrollFrame", nil, frame)
    detailScroll:SetPoint("TOPLEFT", 254, -56)
    detailScroll:SetPoint("BOTTOMRIGHT", -16, 36)
    detailChild = CreateFrame("Frame", nil, detailScroll)
    detailChild:SetSize(430, 200)
    detailScroll:SetScrollChild(detailChild)
    detailScroll:EnableMouseWheel(true)
    detailScroll:SetScript("OnMouseWheel", function(self, delta)
        local nxt = self:GetVerticalScroll() - delta * 28
        local maxScroll = self:GetVerticalScrollRange()
        if nxt < 0 then
            nxt = 0
        end
        if nxt > maxScroll then
            nxt = maxScroll
        end
        self:SetVerticalScroll(nxt)
    end)
    detailFS = detailChild:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    detailFS:SetPoint("TOPLEFT", 4, -4)
    detailFS:SetJustifyH("LEFT")
    detailFS:SetJustifyV("TOP")
    detailFS:SetWordWrap(true)
    detailFS:SetSpacing(2)

    local hint = frame:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    hint:SetPoint("BOTTOMLEFT", 16, 12)
    hint:SetPoint("BOTTOMRIGHT", -16, 12)
    hint:SetJustifyH("LEFT")
    hint:SetText("Målene sjekkes av spillet. Når alle er grønne, tar GutSaga skjermbilde selv.")
    hint:SetTextColor(0.75, 0.7, 0.6)

    if Minimap then
        local btn = CreateFrame("Button", "GutSagaMinimapButton", Minimap)
        btn:SetSize(28, 28)
        btn:SetPoint("TOPLEFT", Minimap, "TOPLEFT", -8, 8)
        btn:SetFrameStrata("MEDIUM")
        local icon = btn:CreateTexture(nil, "BACKGROUND")
        icon:SetAllPoints()
        icon:SetColorTexture(0.45, 0.30, 0.08, 1)
        local letter = btn:CreateFontString(nil, "OVERLAY", "GameFontNormal")
        letter:SetPoint("CENTER")
        letter:SetText("G")
        letter:SetTextColor(1, 0.9, 0.55)
        btn:SetScript("OnClick", toggle)
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
        if C_ChatInfo and C_ChatInfo.RegisterAddonMessagePrefix then
            pcall(C_ChatInfo.RegisterAddonMessagePrefix, PREFIX)
        end
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
        chat("|cffc9a227GutSaga|r: " .. short .. " fullførte et oppdrag.")
    end
end)
