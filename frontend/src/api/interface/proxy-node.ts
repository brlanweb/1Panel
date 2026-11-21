export namespace ProxyNode {
    export interface Item {
        name: string;
        type: string;
        // ms; -1 = failed, 0 = not tested
        delay: number;
        // bytes per second; -1 = failed, 0 = not tested
        speed: number;
        tested: boolean;
    }

    export interface Status {
        subscriptionUrl: string;
        image: string;
        port: number;
        selected: string;
        containerName: string;
        running: boolean;
        proxyUrl: string;
        nodes: Item[];
        testing: boolean;
        testDone: number;
        testTotal: number;
        testedAt: string;
        updatedAt: string;
        message: string;
    }

    export interface Save {
        subscriptionUrl: string;
        image: string;
        port: number;
        selected?: string;
    }
}
