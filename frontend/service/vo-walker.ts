/* tslint:disable */
// hello commonjs - we need some imports - sorted in alphabetical order, by go package
import * as github_com_foomo_walker from './vo-walker'; // frontend/service/vo-walker.ts to frontend/service/vo-walker.ts
// github.com/foomo/walker.FilterOptions
export interface FilterOptions {
	Status:github_com_foomo_walker.StatusStats[];
	MinDur:number;
	MaxDur:number;
}
// github.com/foomo/walker.Filters
export interface Filters {
	Prefix:string;
	Status:number[];
	MinDur:number;
	MaxDur:number;
}
// github.com/foomo/walker.Heading
export interface Heading {
	Level:number;
	Text:string;
}
// github.com/foomo/walker.LinkList
export interface LinkList {
	[index:string]:number;
}
// github.com/foomo/walker.LinkedData
export interface LinkedData {
	@context:string;
	@type:string;
}
// github.com/foomo/walker.ScrapeResult
export interface ScrapeResult {
	TargetURL:string;
	Error:string;
	Code:number;
	Status:string;
	ContentType:string;
	Length:number;
	Links:github_com_foomo_walker.LinkList;
	Duration:number;
	Structure:github_com_foomo_walker.Structure;
}
// github.com/foomo/walker.ServiceStatus
export interface ServiceStatus {
	TargetURL:string;
	Open:number;
	Done:number;
	Pending:number;
}
// github.com/foomo/walker.StatusStats
export interface StatusStats {
	Code:number;
	Count:number;
}
// github.com/foomo/walker.Structure
export interface Structure {
	Title:string;
	Description:string;
	Headings:github_com_foomo_walker.Heading[];
	LinkedData:github_com_foomo_walker.LinkedData[];
	Canonical:string;
	LinkPrev:string;
	LinkNext:string;
}
// end of common js