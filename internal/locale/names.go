package locale

import "strings"

// Names are a language's names of months and days, as date formats show
// them (mmm, mmmm, ddd, dddd) and typed dates may spell them, and its
// words for the two halves of a 12-hour clock.
type Names struct {
	// Months are the months' full names as a date writes them, January
	// first: "września" in "26 września 2026".
	Months [12]string
	// Alone are the full names where a month stands without its day
	// ("wrzesień 2026"), in languages whose form differs; empty where
	// it's Months.
	Alone [12]string
	// Short are the abbreviations mmm shows: "Sep", "sept.", "9月".
	Short [12]string
	// Days are the days' full names, Sunday first; DaysShort their
	// abbreviations, as ddd shows them.
	Days, DaysShort [7]string
	// AM and PM are what AM/PM shows, where the language has words of
	// its own; empty keeps AM and PM.
	AM, PM string
}

// Month is month m's (1 to 12) full name, standing alone or in a date
// with its day.
func (n *Names) Month(m int, alone bool) string {
	if alone && n.Alone[m-1] != "" {
		return n.Alone[m-1]
	}
	return n.Months[m-1]
}

// English are en-US's names, which every English locale shares.
var English = &Names{
	Months: [12]string{"January", "February", "March", "April", "May", "June", "July",
		"August", "September", "October", "November", "December"},
	Short:     [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	Days:      [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	DaysShort: [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
}

// names are the languages' names, by the language of a locale's tag.
// Abbreviations are Excel's where Excel has them, so a sheet shows what
// the same format shows there; a date of numbers stays the more compact.
var names = map[string]*Names{
	"en": English,
	"de": {
		Months: [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli",
			"August", "September", "Oktober", "November", "Dezember"},
		Short:     [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"},
		Days:      [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"},
		DaysShort: [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
	},
	"fr": {
		Months: [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet",
			"août", "septembre", "octobre", "novembre", "décembre"},
		Short:     [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
		Days:      [7]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"},
		DaysShort: [7]string{"dim.", "lun.", "mar.", "mer.", "jeu.", "ven.", "sam."},
	},
	"es": {
		Months: [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
			"agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		Short:     [12]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"},
		Days:      [7]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"},
		DaysShort: [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
	},
	"it": {
		Months: [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio",
			"agosto", "settembre", "ottobre", "novembre", "dicembre"},
		Short:     [12]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"},
		Days:      [7]string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"},
		DaysShort: [7]string{"dom", "lun", "mar", "mer", "gio", "ven", "sab"},
	},
	"pt": {
		Months: [12]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho",
			"agosto", "setembro", "outubro", "novembro", "dezembro"},
		Short: [12]string{"jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"},
		Days: [7]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira",
			"sexta-feira", "sábado"},
		DaysShort: [7]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"},
	},
	"nl": {
		Months: [12]string{"januari", "februari", "maart", "april", "mei", "juni", "juli",
			"augustus", "september", "oktober", "november", "december"},
		Short:     [12]string{"jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"},
		Days:      [7]string{"zondag", "maandag", "dinsdag", "woensdag", "donderdag", "vrijdag", "zaterdag"},
		DaysShort: [7]string{"zo", "ma", "di", "wo", "do", "vr", "za"},
	},
	"sv": {
		Months: [12]string{"januari", "februari", "mars", "april", "maj", "juni", "juli",
			"augusti", "september", "oktober", "november", "december"},
		Short:     [12]string{"jan", "feb", "mar", "apr", "maj", "jun", "jul", "aug", "sep", "okt", "nov", "dec"},
		Days:      [7]string{"söndag", "måndag", "tisdag", "onsdag", "torsdag", "fredag", "lördag"},
		DaysShort: [7]string{"sön", "mån", "tis", "ons", "tors", "fre", "lör"},
	},
	"da": {
		Months: [12]string{"januar", "februar", "marts", "april", "maj", "juni", "juli",
			"august", "september", "oktober", "november", "december"},
		Short:     [12]string{"jan", "feb", "mar", "apr", "maj", "jun", "jul", "aug", "sep", "okt", "nov", "dec"},
		Days:      [7]string{"søndag", "mandag", "tirsdag", "onsdag", "torsdag", "fredag", "lørdag"},
		DaysShort: [7]string{"søn", "man", "tir", "ons", "tor", "fre", "lør"},
	},
	"nb": {
		Months: [12]string{"januar", "februar", "mars", "april", "mai", "juni", "juli",
			"august", "september", "oktober", "november", "desember"},
		Short:     [12]string{"jan", "feb", "mar", "apr", "mai", "jun", "jul", "aug", "sep", "okt", "nov", "des"},
		Days:      [7]string{"søndag", "mandag", "tirsdag", "onsdag", "torsdag", "fredag", "lørdag"},
		DaysShort: [7]string{"søn", "man", "tir", "ons", "tor", "fre", "lør"},
	},
	"fi": {
		Months: [12]string{"tammikuuta", "helmikuuta", "maaliskuuta", "huhtikuuta", "toukokuuta",
			"kesäkuuta", "heinäkuuta", "elokuuta", "syyskuuta", "lokakuuta", "marraskuuta", "joulukuuta"},
		Alone: [12]string{"tammikuu", "helmikuu", "maaliskuu", "huhtikuu", "toukokuu", "kesäkuu",
			"heinäkuu", "elokuu", "syyskuu", "lokakuu", "marraskuu", "joulukuu"},
		Short: [12]string{"tammi", "helmi", "maalis", "huhti", "touko", "kesä", "heinä", "elo",
			"syys", "loka", "marras", "joulu"},
		Days: [7]string{"sunnuntai", "maanantai", "tiistai", "keskiviikko", "torstai", "perjantai",
			"lauantai"},
		DaysShort: [7]string{"su", "ma", "ti", "ke", "to", "pe", "la"},
	},
	"pl": {
		Months: [12]string{"stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca", "lipca",
			"sierpnia", "września", "października", "listopada", "grudnia"},
		Alone: [12]string{"styczeń", "luty", "marzec", "kwiecień", "maj", "czerwiec", "lipiec",
			"sierpień", "wrzesień", "październik", "listopad", "grudzień"},
		Short:     [12]string{"sty", "lut", "mar", "kwi", "maj", "cze", "lip", "sie", "wrz", "paź", "lis", "gru"},
		Days:      [7]string{"niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"},
		DaysShort: [7]string{"niedz.", "pon.", "wt.", "śr.", "czw.", "pt.", "sob."},
	},
	"cs": {
		Months: [12]string{"ledna", "února", "března", "dubna", "května", "června", "července",
			"srpna", "září", "října", "listopadu", "prosince"},
		Alone: [12]string{"leden", "únor", "březen", "duben", "květen", "červen", "červenec",
			"srpen", "září", "říjen", "listopad", "prosinec"},
		Short:     [12]string{"led", "úno", "bře", "dub", "kvě", "čvn", "čvc", "srp", "zář", "říj", "lis", "pro"},
		Days:      [7]string{"neděle", "pondělí", "úterý", "středa", "čtvrtek", "pátek", "sobota"},
		DaysShort: [7]string{"ne", "po", "út", "st", "čt", "pá", "so"},
	},
	"ru": {
		Months: [12]string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля",
			"августа", "сентября", "октября", "ноября", "декабря"},
		Alone: [12]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль",
			"август", "сентябрь", "октябрь", "ноябрь", "декабрь"},
		Short: [12]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"},
		Days: [7]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница",
			"суббота"},
		DaysShort: [7]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"},
	},
	"tr": {
		Months: [12]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz",
			"Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"},
		Short:     [12]string{"Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"},
		Days:      [7]string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"},
		DaysShort: [7]string{"Paz", "Pzt", "Sal", "Çar", "Per", "Cum", "Cmt"},
	},
	"ja": {
		Months:    [12]string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
		Short:     [12]string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
		Days:      [7]string{"日曜日", "月曜日", "火曜日", "水曜日", "木曜日", "金曜日", "土曜日"},
		DaysShort: [7]string{"日", "月", "火", "水", "木", "金", "土"},
		AM:        "午前", PM: "午後",
	},
	"zh": {
		Months: [12]string{"一月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月",
			"十一月", "十二月"},
		Short:     [12]string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
		Days:      [7]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"},
		DaysShort: [7]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"},
		AM:        "上午", PM: "下午",
	},
}

// Names are the locale's names of months and days, in its language;
// English for a nil locale.
func (l *Locale) Names() *Names {
	if l == nil || l.names == nil {
		return English
	}
	return l.names
}

// Each locale finds its language's names once, as dates are drawn with
// them every frame.
func init() {
	for i := range table {
		lang, _, _ := strings.Cut(table[i].Tag, "-")
		table[i].names = names[lang]
	}
}
