/* tslint:disable */
// hello commonjs - we need some imports - sorted in alphabetical order, by go package
import * as github_com_foomo_walker from './vo-walker'; // frontend/service/service.ts to frontend/service/vo-walker.ts

export class ServiceClient {
	public static defaultEndpoint = "/service/walker";
	constructor(
		public transport:<T>(method: string, data?: any[]) => Promise<T>
	) {}
	async getResults(filters:github_com_foomo_walker.Filters, page:number, pageSize:number):Promise<{filterOptions:github_com_foomo_walker.FilterOptions; results:github_com_foomo_walker.ScrapeResult[]; numPages:number}> {
		let response = await this.transport<{0:github_com_foomo_walker.FilterOptions; 1:github_com_foomo_walker.ScrapeResult[]; 2:number}>("GetResults", [filters, page, pageSize])
		let responseObject = {filterOptions : response[0], results : response[1], numPages : response[2]};
		return responseObject;
	}
	async getStatus():Promise<github_com_foomo_walker.ServiceStatus> {
		return (await this.transport<{0:github_com_foomo_walker.ServiceStatus}>("GetStatus", []))[0]
	}
}